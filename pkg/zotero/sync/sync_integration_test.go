package sync

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	stdSync "sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/info"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

const (
	defaultLocalEndpoint = "http://localhost:23119/api"
	defaultCloudEndpoint = "https://api.zotero.org"
	defaultTestGroupId   = int64(6642571)
)

func loadEnv() {
	if os.Getenv("DATABASE_URL") != "" {
		return
	}
	candidates := []string{".env", "../.env", "../../.env", "../../../.env"}
	for _, c := range candidates {
		absPath, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		f, err := os.Open(absPath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				if os.Getenv(k) == "" {
					os.Setenv(k, v)
				}
			}
		}
		f.Close()
		if os.Getenv("DATABASE_URL") != "" {
			break
		}
	}
}

func getIntegrationTestStorage(t *testing.T, groupID int64) (*storage.Storage, *pgxpool.Pool, string, func()) {
	t.Helper()
	loadEnv()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping database integration test")
		return nil, nil, "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Skipf("cannot parse database url (%v); skipping integration test", err)
		return nil, nil, "", nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database (%v); skipping integration test", err)
		return nil, nil, "", nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("database ping failed (%v); skipping integration test", err)
		return nil, nil, "", nil
	}

	// Clean up any existing data for groupID before running the test
	bgCtx := context.Background()
	_, _ = pool.Exec(bgCtx, "DELETE FROM tags WHERE library=$1", groupID)
	_, _ = pool.Exec(bgCtx, "DELETE FROM items WHERE library=$1", groupID)
	_, _ = pool.Exec(bgCtx, "DELETE FROM collections WHERE library=$1", groupID)
	_, _ = pool.Exec(bgCtx, "DELETE FROM syncgroups WHERE id=$1", groupID)
	_, _ = pool.Exec(bgCtx, "DELETE FROM groups WHERE id=$1", groupID)

	zlog := zerolog.Nop()
	st := storage.NewStorage(pool, true, &zlog)

	cleanup := func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM tags WHERE library=$1", groupID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM items WHERE library=$1", groupID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM collections WHERE library=$1", groupID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM syncgroups WHERE id=$1", groupID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM groups WHERE id=$1", groupID)
		pool.Close()
	}

	return st, pool, "", cleanup
}

func getLocalTestConfig() (endpoint string, groupId int64, localKey string, cloudKey string) {
	loadEnv()
	endpoint = os.Getenv("ZOTERO_LOCAL_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultLocalEndpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	groupId = defaultTestGroupId
	if grpEnv := os.Getenv("ZOTERO_TEST_GROUP"); grpEnv != "" {
		if gid, err := strconv.ParseInt(grpEnv, 10, 64); err == nil {
			groupId = gid
		}
	}

	localKey = os.Getenv("ZOTERO_LOCAL_KEY")
	cloudKey = os.Getenv("ZOTERO_API_KEY")
	return endpoint, groupId, localKey, cloudKey
}

var (
	localAuthMutex stdSync.Mutex
	cachedLocalKey string
)

func ensureLocalWriteAuth(t *testing.T, cl *client.Client, endpoint, localKey, cloudKey string) (bool, error) {
	t.Helper()
	if cl.GetApiKey() != "" {
		return true, nil
	}

	isLocalhost := strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1")
	if localKey != "" {
		cl.SetApiKey(localKey)
		return true, nil
	}
	if !isLocalhost {
		if cloudKey != "" {
			cl.SetApiKey(cloudKey)
			return true, nil
		}
		return false, fmt.Errorf("no API key configured for remote endpoint %s", endpoint)
	}

	// Localhost without key
	if os.Getenv("CI") != "" {
		return false, fmt.Errorf("CI environment detected without ZOTERO_LOCAL_KEY")
	}

	localAuthMutex.Lock()
	defer localAuthMutex.Unlock()

	if cachedLocalKey != "" {
		cl.SetApiKey(cachedLocalKey)
		return true, nil
	}

	t.Logf("Local write authorization required: requesting local token from Zotero desktop...")
	authCtx, authCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer authCancel()
	key, authErr := cl.AuthorizeLocal(authCtx, "ZSyncIntegrationTest")
	if authErr != nil || key == "" {
		return false, authErr
	}

	cachedLocalKey = key
	cl.SetApiKey(key)
	t.Logf("Successfully obtained and cached local write authorization key: %s", key)
	return true, nil
}

func checkLocalZoteroAvailable(t *testing.T, endpoint string, groupId int64) {
	t.Helper()
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	probeUrl := fmt.Sprintf("%s/groups/%d", endpoint, groupId)
	req, err := http.NewRequest(http.MethodGet, probeUrl, nil)
	if err != nil {
		t.Fatalf("failed to construct probe request: %v", err)
	}
	req.Header.Set("User-Agent", info.GetUserAgent())

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Local Zotero instance unreachable at %s (%v) - skipping local integration test", probeUrl, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
		t.Skipf("Local Zotero responded with status %d at %s - skipping local integration test", resp.StatusCode, probeUrl)
		return
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Skipf("Local Zotero responded with status %d for group %d at %s - skipping local integration test", resp.StatusCode, groupId, probeUrl)
		return
	}
}

func getCloudTestConfig() (endpoint string, groupId int64, apiKey string) {
	loadEnv()
	endpoint = os.Getenv("ZOTERO_CLOUD_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultCloudEndpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	groupId = defaultTestGroupId
	if grpEnv := os.Getenv("ZOTERO_TEST_GROUP"); grpEnv != "" {
		if gid, err := strconv.ParseInt(grpEnv, 10, 64); err == nil {
			groupId = gid
		}
	}

	apiKey = os.Getenv("ZOTERO_API_KEY")
	return endpoint, groupId, apiKey
}

func checkCloudZoteroAvailable(t *testing.T, endpoint string, groupId int64, apiKey string) {
	t.Helper()
	if apiKey == "" {
		t.Skip("ZOTERO_API_KEY is not set - skipping cloud integration test")
		return
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	probeUrl := fmt.Sprintf("%s/groups/%d/items?limit=1", endpoint, groupId)
	req, err := http.NewRequest(http.MethodGet, probeUrl, nil)
	if err != nil {
		t.Fatalf("failed to construct probe request: %v", err)
	}
	req.Header.Set("User-Agent", info.GetUserAgent())
	req.Header.Set("Zotero-API-Version", "3")
	req.Header.Set("Zotero-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Zotero Cloud API unreachable at %s (%v) - skipping cloud integration test", probeUrl, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		t.Skipf("Zotero Cloud API returned %d for group %d - skipping cloud integration test", resp.StatusCode, groupId)
		return
	}
	if resp.StatusCode >= 500 {
		t.Skipf("Zotero Cloud API returned server error %d - skipping cloud integration test", resp.StatusCode)
		return
	}
}

func TestSyncer_Integration_Database_LocalClient(t *testing.T) {
	endpoint, groupID, localKey, _ := getLocalTestConfig()
	checkLocalZoteroAvailable(t, endpoint, groupID)

	st, _, _, cleanupDB := getIntegrationTestStorage(t, groupID)
	defer cleanupDB()

	zlog := zerolog.Nop()
	cl, err := client.NewClient(testCtx, endpoint, localKey, &zlog)
	if err != nil {
		t.Skipf("cannot create local client: %v", err)
		return
	}

	group, err := cl.GetGroup(testCtx, groupID)
	if err != nil || group == nil {
		t.Skipf("failed to get group %d from local zotero: %v - skipping integration test", groupID, err)
		return
	}

	opLogger := logging.MustGetLogger("test")
	tempDir := t.TempDir()
	fsDir := filepath.Join(tempDir, "storage")
	if err := os.MkdirAll(fsDir, 0755); err != nil {
		t.Fatalf("failed to create fsDir: %v", err)
	}
	fs, err := filesystem.NewLocalFs(fsDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create local fs: %v", err)
	}

	syncer := NewSyncer(cl, st, fs, &zlog)

	// Configure group record in database
	group.Active = true
	group.Direction = model.SyncDirection_BothLocal
	group.SyncTags = true
	group.CollectionVersion = 0
	group.ItemVersion = 0
	group.TagVersion = 0

	if err := st.UpdateGroup(testCtx, group); err != nil {
		t.Fatalf("failed to create/update test group in database: %v", err)
	}

	if err := syncer.SyncGroup(testCtx, group); err != nil {
		t.Fatalf("SyncGroup for local client failed: %v", err)
	}

	// Test BackupLocal with filesystem
	backupDir := filepath.Join(tempDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backupDir: %v", err)
	}
	backupFs, err := filesystem.NewLocalFs(backupDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create backup fs: %v", err)
	}

	if err := syncer.BackupLocal(testCtx, group, backupFs); err != nil {
		t.Fatalf("BackupLocal failed: %v", err)
	}

	groupFolder := fmt.Sprintf("%d", groupID)
	groupExists, err := backupFs.FileExists(groupFolder, "group.json")
	if err != nil || !groupExists {
		t.Errorf("expected backup file %s/group.json to exist", groupFolder)
	}
}

func TestSyncer_Integration_Database_CloudClient(t *testing.T) {
	endpoint, groupID, apiKey := getCloudTestConfig()
	checkCloudZoteroAvailable(t, endpoint, groupID, apiKey)

	st, _, _, cleanupDB := getIntegrationTestStorage(t, groupID)
	defer cleanupDB()

	zlog := zerolog.Nop()
	cl, err := client.NewClient(testCtx, endpoint, apiKey, &zlog)
	if err != nil {
		t.Skipf("cannot create cloud client: %v", err)
		return
	}

	group, err := cl.GetGroup(testCtx, groupID)
	if err != nil || group == nil {
		t.Skipf("failed to get group %d from cloud zotero: %v - skipping integration test", groupID, err)
		return
	}

	opLogger := logging.MustGetLogger("test")
	tempDir := t.TempDir()
	fsDir := filepath.Join(tempDir, "storage")
	if err := os.MkdirAll(fsDir, 0755); err != nil {
		t.Fatalf("failed to create fsDir: %v", err)
	}
	fs, err := filesystem.NewLocalFs(fsDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create local fs: %v", err)
	}

	syncer := NewSyncer(cl, st, fs, &zlog)

	// Configure group record in database
	group.Active = true
	group.Direction = model.SyncDirection_BothCloud
	group.SyncTags = true
	group.CollectionVersion = 0
	group.ItemVersion = 0
	group.TagVersion = 0

	if err := st.UpdateGroup(testCtx, group); err != nil {
		t.Fatalf("failed to create/update test group in database: %v", err)
	}

	if err := syncer.SyncGroup(testCtx, group); err != nil {
		t.Fatalf("SyncGroup for cloud client failed: %v", err)
	}

	// Test BackupLocal with filesystem
	backupDir := filepath.Join(tempDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backupDir: %v", err)
	}
	backupFs, err := filesystem.NewLocalFs(backupDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create backup fs: %v", err)
	}

	if err := syncer.BackupLocal(testCtx, group, backupFs); err != nil {
		t.Fatalf("BackupLocal failed: %v", err)
	}

	groupFolder := fmt.Sprintf("%d", groupID)
	groupExists, err := backupFs.FileExists(groupFolder, "group.json")
	if err != nil || !groupExists {
		t.Errorf("expected backup file %s/group.json to exist", groupFolder)
	}
}

func TestSyncer_Integration_Bidirectional_LocalClient(t *testing.T) {
	endpoint, groupID, localKey, cloudKey := getLocalTestConfig()
	checkLocalZoteroAvailable(t, endpoint, groupID)

	st, _, _, cleanupDB := getIntegrationTestStorage(t, groupID)
	defer cleanupDB()

	zlog := zerolog.Nop()
	cl, err := client.NewClient(testCtx, endpoint, localKey, &zlog)
	if err != nil {
		t.Skipf("cannot create local client: %v", err)
		return
	}

	group, err := cl.GetGroup(testCtx, groupID)
	if err != nil || group == nil {
		t.Skipf("failed to get group %d from local zotero: %v - skipping integration test", groupID, err)
		return
	}

	hasWriteAuth, authErr := ensureLocalWriteAuth(t, cl, endpoint, localKey, cloudKey)
	if !hasWriteAuth {
		t.Logf("Local Zotero write authorization not available (%v). Subtests requiring write access will be skipped.", authErr)
	}

	opLogger := logging.MustGetLogger("test")
	tempDir := t.TempDir()
	fsDir := filepath.Join(tempDir, "storage")
	if err := os.MkdirAll(fsDir, 0755); err != nil {
		t.Fatalf("failed to create fsDir: %v", err)
	}
	fs, err := filesystem.NewLocalFs(fsDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create local fs: %v", err)
	}

	syncer := NewSyncer(cl, st, fs, &zlog)

	// Configure group record in database
	group.Active = true
	group.Direction = model.SyncDirection_BothLocal
	group.SyncTags = true
	group.CollectionVersion = 0
	group.ItemVersion = 0
	group.TagVersion = 0

	if err := st.UpdateGroup(testCtx, group); err != nil {
		t.Fatalf("failed to create/update test group in database: %v", err)
	}

	// Initial sync
	if err := syncer.SyncGroup(testCtx, group); err != nil {
		t.Fatalf("initial SyncGroup failed: %v", err)
	}

	// -----------------------------------------------------------------
	// 1. Zotero -> Database: Write item in Zotero, sync, verify in DB
	// -----------------------------------------------------------------
	t.Run("ZoteroToDatabase", func(t *testing.T) {
		if !hasWriteAuth {
			t.Skipf("Local Zotero API write operations require authorization: set ZOTERO_LOCAL_KEY or authorize in Zotero desktop: %v", authErr)
			return
		}

		zotItemUnique := model.CreateKey()
		zotTitle := fmt.Sprintf("IntegrationTest Local Zotero2DB %s", zotItemUnique)
		zotItemData := model.ItemGeneric{
			ItemDataBase: model.ItemDataBase{
				ItemType: "book",
				Tags: []model.ItemTag{
					{Tag: "integration-test-local-zotero2db"},
				},
				Creators: []model.ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Alan",
						LastName:    "Turing",
					},
				},
			},
			Title: zotTitle,
		}

		var lastMod int64 = group.ItemVersion
		createRes, err := cl.CreateItems(testCtx, groupID, []model.ItemGeneric{zotItemData}, &lastMod)
		if err != nil {
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				t.Skipf("Local Zotero API key does not have write permissions: %v", err)
				return
			}
			t.Fatalf("failed to create item in Local Zotero: %v", err)
		}
		createdKey, err := createRes.CheckSuccess(0)
		if err != nil {
			if len(createRes.Failed) > 0 {
				t.Fatalf("create item in Local Zotero failed: %v", createRes.Failed)
			}
			t.Fatalf("create item in Local Zotero returned no successful items: %v", err)
		}
		defer func() {
			_ = cl.DeleteItem(testCtx, groupID, createdKey, 0)
		}()

		// Run Sync
		if err := syncer.SyncGroup(testCtx, group); err != nil {
			t.Fatalf("SyncGroup failed: %v", err)
		}

		// Verify in Database
		items, err := st.GetItemsByKey(testCtx, groupID, []string{createdKey})
		if err != nil {
			t.Fatalf("st.GetItemsByKey failed: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item in database for key %s, found %d", createdKey, len(items))
		}
		dbItem := items[0]
		if dbItem.Key != createdKey {
			t.Errorf("expected DB item key %s, got %s", createdKey, dbItem.Key)
		}
		if dbItem.Data.Title != zotTitle {
			t.Errorf("expected DB item title '%s', got '%s'", zotTitle, dbItem.Data.Title)
		}
		if dbItem.Status != model.SyncStatus_Synced {
			t.Errorf("expected DB item status Synced, got %v", dbItem.Status)
		}
	})

	// -----------------------------------------------------------------
	// 2. Database -> Zotero: Modify/Write item in DB, sync, verify in Zotero
	// -----------------------------------------------------------------
	t.Run("DatabaseToZotero", func(t *testing.T) {
		if !hasWriteAuth {
			t.Skipf("Local Zotero API write operations require authorization: set ZOTERO_LOCAL_KEY or authorize in Zotero desktop: %v", authErr)
			return
		}

		baseUnique := model.CreateKey()
		baseTitle := fmt.Sprintf("IntegrationTest Local DB2Zotero Initial %s", baseUnique)
		baseItem := model.ItemGeneric{
			ItemDataBase: model.ItemDataBase{
				ItemType: "book",
				Tags: []model.ItemTag{
					{Tag: "integration-test-local-db2zotero"},
				},
				Creators: []model.ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Ada",
						LastName:    "Lovelace",
					},
				},
			},
			Title: baseTitle,
		}

		var lastMod int64 = group.ItemVersion
		createRes, err := cl.CreateItems(testCtx, groupID, []model.ItemGeneric{baseItem}, &lastMod)
		if err != nil {
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				t.Skipf("Local Zotero API key does not have write permissions: %v", err)
				return
			}
			t.Fatalf("failed to create base item in Local Zotero: %v", err)
		}
		createdKey, err := createRes.CheckSuccess(0)
		if err != nil {
			if len(createRes.Failed) > 0 {
				t.Fatalf("create base item in Local Zotero failed: %v", createRes.Failed)
			}
			t.Fatalf("create base item in Local Zotero returned no successful items: %v", err)
		}
		defer func() {
			_ = cl.DeleteItem(testCtx, groupID, createdKey, 0)
		}()

		// Sync down to DB
		if err := syncer.SyncGroup(testCtx, group); err != nil {
			t.Fatalf("SyncGroup failed: %v", err)
		}

		// Verify item exists in DB
		dbItems, err := st.GetItemsByKey(testCtx, groupID, []string{createdKey})
		if err != nil || len(dbItems) == 0 {
			t.Fatalf("failed to fetch synced item from DB: %v", err)
		}

		itemToModify := dbItems[0]
		updatedTitle := fmt.Sprintf("IntegrationTest Local DB2Zotero Updated %s", createdKey)
		itemToModify.Data.Title = updatedTitle
		itemToModify.Status = model.SyncStatus_Modified

		// Update in DB
		if err := st.UpdateItem(testCtx, groupID, &itemToModify); err != nil {
			t.Fatalf("failed to update item in DB: %v", err)
		}

		// Sync to Zotero
		if err := syncer.SyncGroup(testCtx, group); err != nil {
			t.Fatalf("SyncGroup upload failed: %v", err)
		}

		// Verify in Zotero
		zotItem, err := cl.GetItemByKey(testCtx, groupID, createdKey)
		if err != nil {
			t.Fatalf("failed to get item from Zotero: %v", err)
		}
		if zotItem == nil {
			t.Fatalf("expected item %s in Zotero, got nil", createdKey)
		}
		if zotItem.Data.Title != updatedTitle {
			t.Errorf("expected Zotero item title '%s', got '%s'", updatedTitle, zotItem.Data.Title)
		}

		// Verify in DB that status transitioned to Synced and title matches
		afterSyncItems, err := st.GetItemsByKey(testCtx, groupID, []string{createdKey})
		if err != nil || len(afterSyncItems) == 0 {
			t.Fatalf("failed to get item from DB after sync: %v", err)
		}
		afterSyncItem := afterSyncItems[0]
		if afterSyncItem.Status != model.SyncStatus_Synced {
			t.Errorf("expected DB item status Synced, got %v", afterSyncItem.Status)
		}
		if afterSyncItem.Data.Title != updatedTitle {
			t.Errorf("expected DB item title '%s', got '%s'", updatedTitle, afterSyncItem.Data.Title)
		}
	})
}

func TestSyncer_Integration_Bidirectional_CloudClient(t *testing.T) {
	endpoint, groupID, apiKey := getCloudTestConfig()
	checkCloudZoteroAvailable(t, endpoint, groupID, apiKey)

	st, _, _, cleanupDB := getIntegrationTestStorage(t, groupID)
	defer cleanupDB()

	zlog := zerolog.Nop()
	cl, err := client.NewClient(testCtx, endpoint, apiKey, &zlog)
	if err != nil {
		t.Skipf("cannot create cloud client: %v", err)
		return
	}

	group, err := cl.GetGroup(testCtx, groupID)
	if err != nil || group == nil {
		t.Skipf("failed to get group %d from cloud zotero: %v - skipping integration test", groupID, err)
		return
	}

	opLogger := logging.MustGetLogger("test")
	tempDir := t.TempDir()
	fsDir := filepath.Join(tempDir, "storage")
	if err := os.MkdirAll(fsDir, 0755); err != nil {
		t.Fatalf("failed to create fsDir: %v", err)
	}
	fs, err := filesystem.NewLocalFs(fsDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create local fs: %v", err)
	}

	syncer := NewSyncer(cl, st, fs, &zlog)

	// Configure group record in database
	group.Active = true
	group.Direction = model.SyncDirection_BothCloud
	group.SyncTags = true
	group.CollectionVersion = 0
	group.ItemVersion = 0
	group.TagVersion = 0

	if err := st.UpdateGroup(testCtx, group); err != nil {
		t.Fatalf("failed to create/update test group in database: %v", err)
	}

	// Initial full sync
	if err := syncer.SyncGroup(testCtx, group); err != nil {
		t.Fatalf("initial SyncGroup failed: %v", err)
	}

	// -----------------------------------------------------------------
	// 1. Zotero -> Database: Write item in Zotero, sync, verify in DB
	// -----------------------------------------------------------------
	t.Run("ZoteroToDatabase", func(t *testing.T) {
		zotItemUnique := model.CreateKey()
		zotTitle := fmt.Sprintf("IntegrationTest Zotero2DB %s", zotItemUnique)
		zotItemData := model.ItemGeneric{
			ItemDataBase: model.ItemDataBase{
				ItemType: "book",
				Tags: []model.ItemTag{
					{Tag: "integration-test-zotero2db"},
				},
				Creators: []model.ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Alan",
						LastName:    "Turing",
					},
				},
			},
			Title: zotTitle,
		}

		var lastMod int64 = group.ItemVersion
		createRes, err := cl.CreateItems(testCtx, groupID, []model.ItemGeneric{zotItemData}, &lastMod)
		if err != nil {
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				t.Skipf("Zotero API key does not have write permissions: %v", err)
				return
			}
			t.Fatalf("failed to create item in Zotero: %v", err)
		}
		createdKey, err := createRes.CheckSuccess(0)
		if err != nil {
			if len(createRes.Failed) > 0 {
				t.Fatalf("create item in Zotero failed: %v", createRes.Failed)
			}
			t.Fatalf("create item in Zotero returned no successful items: %v", err)
		}
		defer func() {
			_ = cl.DeleteItem(testCtx, groupID, createdKey, 0)
		}()

		// Run Sync
		if err := syncer.SyncGroup(testCtx, group); err != nil {
			t.Fatalf("SyncGroup failed: %v", err)
		}

		// Verify in Database
		items, err := st.GetItemsByKey(testCtx, groupID, []string{createdKey})
		if err != nil {
			t.Fatalf("st.GetItemsByKey failed: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item in database for key %s, found %d", createdKey, len(items))
		}
		dbItem := items[0]
		if dbItem.Key != createdKey {
			t.Errorf("expected DB item key %s, got %s", createdKey, dbItem.Key)
		}
		if dbItem.Data.Title != zotTitle {
			t.Errorf("expected DB item title '%s', got '%s'", zotTitle, dbItem.Data.Title)
		}
		if dbItem.Status != model.SyncStatus_Synced {
			t.Errorf("expected DB item status Synced, got %v", dbItem.Status)
		}
	})

	// -----------------------------------------------------------------
	// 2. Database -> Zotero: Modify/Write item in DB, sync, verify in Zotero
	// -----------------------------------------------------------------
	t.Run("DatabaseToZotero", func(t *testing.T) {
		baseUnique := model.CreateKey()
		baseTitle := fmt.Sprintf("IntegrationTest DB2Zotero Initial %s", baseUnique)
		baseItem := model.ItemGeneric{
			ItemDataBase: model.ItemDataBase{
				ItemType: "book",
				Tags: []model.ItemTag{
					{Tag: "integration-test-db2zotero"},
				},
				Creators: []model.ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Ada",
						LastName:    "Lovelace",
					},
				},
			},
			Title: baseTitle,
		}

		var lastMod int64 = group.ItemVersion
		createRes, err := cl.CreateItems(testCtx, groupID, []model.ItemGeneric{baseItem}, &lastMod)
		if err != nil {
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				t.Skipf("Zotero API key does not have write permissions: %v", err)
				return
			}
			t.Fatalf("failed to create base item in Zotero: %v", err)
		}
		createdKey, err := createRes.CheckSuccess(0)
		if err != nil {
			if len(createRes.Failed) > 0 {
				t.Fatalf("create base item in Zotero failed: %v", createRes.Failed)
			}
			t.Fatalf("create base item in Zotero returned no successful items: %v", err)
		}
		defer func() {
			_ = cl.DeleteItem(testCtx, groupID, createdKey, 0)
		}()

		// Sync down to DB
		if err := syncer.SyncGroup(testCtx, group); err != nil {
			t.Fatalf("SyncGroup failed: %v", err)
		}

		// Verify item exists in DB
		dbItems, err := st.GetItemsByKey(testCtx, groupID, []string{createdKey})
		if err != nil || len(dbItems) == 0 {
			t.Fatalf("failed to fetch synced item from DB: %v", err)
		}

		itemToModify := dbItems[0]
		updatedTitle := fmt.Sprintf("IntegrationTest DB2Zotero Updated %s", createdKey)
		itemToModify.Data.Title = updatedTitle
		itemToModify.Status = model.SyncStatus_Modified

		// Update in DB
		if err := st.UpdateItem(testCtx, groupID, &itemToModify); err != nil {
			t.Fatalf("failed to update item in DB: %v", err)
		}

		// Verify it is modified in DB before sync
		modItems, err := st.GetModifiedItems(testCtx, groupID)
		if err != nil {
			t.Fatalf("st.GetModifiedItems failed: %v", err)
		}
		foundMod := false
		for _, mi := range modItems {
			if mi.Key == createdKey {
				foundMod = true
				break
			}
		}
		if !foundMod {
			t.Fatalf("expected item %s to be in modified items before sync", createdKey)
		}

		// Sync to Zotero
		if err := syncer.SyncGroup(testCtx, group); err != nil {
			t.Fatalf("SyncGroup upload failed: %v", err)
		}

		// Verify in Zotero
		zotItem, err := cl.GetItemByKey(testCtx, groupID, createdKey)
		if err != nil {
			t.Fatalf("failed to get item from Zotero: %v", err)
		}
		if zotItem == nil {
			t.Fatalf("expected item %s in Zotero, got nil", createdKey)
		}
		if zotItem.Data.Title != updatedTitle {
			t.Errorf("expected Zotero item title '%s', got '%s'", updatedTitle, zotItem.Data.Title)
		}

		// Verify in DB that status transitioned to Synced and title matches
		afterSyncItems, err := st.GetItemsByKey(testCtx, groupID, []string{createdKey})
		if err != nil || len(afterSyncItems) == 0 {
			t.Fatalf("failed to get item from DB after sync: %v", err)
		}
		afterSyncItem := afterSyncItems[0]
		if afterSyncItem.Status != model.SyncStatus_Synced {
			t.Errorf("expected DB item status Synced, got %v", afterSyncItem.Status)
		}
		if afterSyncItem.Data.Title != updatedTitle {
			t.Errorf("expected DB item title '%s', got '%s'", updatedTitle, afterSyncItem.Data.Title)
		}
	})
}
