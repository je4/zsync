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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

	schema := "public"
	if s := os.Getenv("DATABASE_SCHEMA"); s != "" {
		schema = s
	}

	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	if _, ok := cfg.ConnConfig.RuntimeParams["search_path"]; !ok {
		cfg.ConnConfig.RuntimeParams["search_path"] = fmt.Sprintf("%s, public", schema)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf("SET search_path = %s, public", schema))
		return err
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
	_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.tags WHERE library=$1", schema), groupID)
	_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.items WHERE library=$1", schema), groupID)
	_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.collections WHERE library=$1", schema), groupID)
	_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.syncgroups WHERE id=$1", schema), groupID)
	_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.groups WHERE id=$1", schema), groupID)

	zlog := zerolog.Nop()
	st := storage.NewStorage(pool, schema, true, &zlog)

	cleanup := func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM %s.tags WHERE library=$1", schema), groupID)
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM %s.items WHERE library=$1", schema), groupID)
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM %s.collections WHERE library=$1", schema), groupID)
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM %s.syncgroups WHERE id=$1", schema), groupID)
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM %s.groups WHERE id=$1", schema), groupID)
		pool.Close()
	}

	return st, pool, schema, cleanup
}

func getLocalTestConfig() (endpoint string, groupId int64, localKey string, cloudKey string) {
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
	cl, err := client.NewClient(endpoint, localKey, &zlog)
	if err != nil {
		t.Skipf("cannot create local client: %v", err)
		return
	}

	group, err := cl.GetGroup(groupID)
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

	if err := st.UpdateGroup(group); err != nil {
		t.Fatalf("failed to create/update test group in database: %v", err)
	}

	if err := syncer.SyncGroup(group); err != nil {
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

	if err := syncer.BackupLocal(group, backupFs); err != nil {
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
	cl, err := client.NewClient(endpoint, apiKey, &zlog)
	if err != nil {
		t.Skipf("cannot create cloud client: %v", err)
		return
	}

	group, err := cl.GetGroup(groupID)
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

	if err := st.UpdateGroup(group); err != nil {
		t.Fatalf("failed to create/update test group in database: %v", err)
	}

	if err := syncer.SyncGroup(group); err != nil {
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

	if err := syncer.BackupLocal(group, backupFs); err != nil {
		t.Fatalf("BackupLocal failed: %v", err)
	}

	groupFolder := fmt.Sprintf("%d", groupID)
	groupExists, err := backupFs.FileExists(groupFolder, "group.json")
	if err != nil || !groupExists {
		t.Errorf("expected backup file %s/group.json to exist", groupFolder)
	}
}
