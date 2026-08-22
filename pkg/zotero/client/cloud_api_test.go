package client

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/je4/zsync/v2/info"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/rs/zerolog"
)

const (
	defaultCloudEndpoint = "https://api.zotero.org"
)

func loadCloudTestEnv() {
	candidates := []string{".env", "../.env", "../../.env", "../../../.env"}
	for _, candidate := range candidates {
		path, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, value)
			}
		}
		_ = file.Close()
	}
}

func getCloudTestConfig() (string, int64, string) {
	loadCloudTestEnv()

	endpoint := os.Getenv("ZOTERO_CLOUD_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultCloudEndpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	groupId := defaultTestGroupId
	if grpEnv := os.Getenv("ZOTERO_TEST_GROUP"); grpEnv != "" {
		if gid, err := strconv.ParseInt(grpEnv, 10, 64); err == nil {
			groupId = gid
		}
	}

	apiKey := os.Getenv("ZOTERO_API_KEY")
	return endpoint, groupId, apiKey
}

func checkCloudZoteroAvailable(t *testing.T, endpoint string, groupId int64, apiKey string) {
	t.Helper()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	probeUrl := fmt.Sprintf("%s/groups/%d/items?limit=1", endpoint, groupId)

	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodGet, probeUrl, nil)
		if err != nil {
			t.Fatalf("failed to construct probe request: %v", err)
		}
		req.Header.Set("User-Agent", info.GetUserAgent())
		req.Header.Set("Zotero-API-Version", "3")
		if apiKey != "" {
			req.Header.Set("Zotero-API-Key", apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Logf("Cloud Zotero API unreachable at %s (%v). Skipping integration test.", probeUrl, err)
			t.Skipf("Cloud Zotero API is not available at %s - skipping test", endpoint)
			return
		}

		// Check Backoff header
		if backoffStr := strings.TrimSpace(resp.Header.Get("Backoff")); backoffStr != "" {
			if backoffVal, bErr := strconv.ParseInt(backoffStr, 10, 64); bErr == nil && backoffVal > 0 {
				t.Logf("Cloud Zotero API requested backoff %d seconds during probe", backoffVal)
				time.Sleep(time.Duration(backoffVal) * time.Second)
			}
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			retryAfterStr := strings.TrimSpace(resp.Header.Get("Retry-After"))
			var waitSec int64 = 2
			if val, rErr := strconv.ParseInt(retryAfterStr, 10, 64); rErr == nil && val > 0 {
				waitSec = val
			} else if parsedTime, pErr := http.ParseTime(retryAfterStr); pErr == nil {
				diff := time.Until(parsedTime)
				if diff > 0 {
					waitSec = int64(diff/time.Second) + 1
				}
			}
			resp.Body.Close()
			t.Logf("Cloud Zotero probe received status %d on attempt %d, waiting %d seconds (Retry-After)", resp.StatusCode, attempt, waitSec)
			time.Sleep(time.Duration(waitSec) * time.Second)
			continue
		}

		resp.Body.Close()
		t.Logf("Cloud Zotero API returned status %d at %s. Skipping integration test.", resp.StatusCode, probeUrl)
		t.Skipf("Cloud Zotero API returned status %d at %s - skipping test", resp.StatusCode, probeUrl)
		return
	}

	t.Skipf("Cloud Zotero API rate limited or unavailable after %d attempts at %s - skipping test", maxAttempts, probeUrl)
}

func getCloudTestClient(t *testing.T) (*Client, *model.Group) {
	t.Helper()

	endpoint, groupId, apiKey := getCloudTestConfig()
	checkCloudZoteroAvailable(t, endpoint, groupId, apiKey)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewClient(testCtx, endpoint, apiKey, &logger)
	if err != nil {
		t.Fatalf("failed to create authenticated Cloud Zotero client: %v", err)
	}

	group, err := zot.GetGroup(testCtx, groupId)
	if err != nil {
		t.Fatalf("failed to retrieve cloud test group %d: %v", groupId, err)
	}
	if group == nil {
		t.Fatalf("group %d not found on Cloud Zotero", groupId)
	}

	return zot, group
}

func TestCloudApi_PreFlightAndClientInit(t *testing.T) {
	zot, group := getCloudTestClient(t)
	if zot == nil {
		t.Fatal("expected non-nil Zotero client")
	}
	if group == nil {
		t.Fatal("expected non-nil Group")
	}
	if group.Id != defaultTestGroupId && group.Data.Name != defaultTestGroupName {
		t.Errorf("expected group ID %d or name %s, got ID %d name %s", defaultTestGroupId, defaultTestGroupName, group.Id, group.Data.Name)
	}
}

func TestCloudApi_ReadAPITESTGroup(t *testing.T) {
	_, group := getCloudTestClient(t)

	if group.Id != defaultTestGroupId {
		t.Errorf("expected group ID %d, got %d", defaultTestGroupId, group.Id)
	}
	if group.Data.Name != defaultTestGroupName {
		t.Errorf("expected group name '%s', got '%s'", defaultTestGroupName, group.Data.Name)
	}
	if group.Version <= 0 {
		t.Errorf("expected positive group version, got %d", group.Version)
	}
}

func TestCloudApi_ReadUserGroupVersions(t *testing.T) {
	zot, group := getCloudTestClient(t)
	if zot.CurrentKey == nil || zot.CurrentKey.UserId == 0 {
		t.Skip("ZOTERO_API_KEY with a resolvable user ID is required to read user group versions")
	}

	versions, err := zot.GetUserGroupVersions(testCtx, zot.CurrentKey)
	if err != nil {
		t.Fatalf("failed to retrieve cloud user group versions: %v", err)
	}
	if versions == nil {
		t.Fatal("expected non-nil user group versions map")
	}

	version, ok := versions[group.Id]
	if !ok {
		t.Fatalf("expected APITEST group %d in user group versions, got %v", group.Id, versions)
	}
	if version <= 0 {
		t.Errorf("expected positive version for group %d, got %d", group.Id, version)
	}
	if version != group.Version {
		t.Errorf("expected version %d for group %d, got %d", group.Version, group.Id, version)
	}
}

func TestCloudApi_ReadAPITESTItems(t *testing.T) {
	zot, group := getCloudTestClient(t)

	items, resp, err := zot.GetItemsQuery(testCtx, group.Id, map[string]string{"limit": "10"})
	if err != nil {
		t.Fatalf("failed to query cloud items for group %d: %v", group.Id, err)
	}
	if items == nil {
		t.Fatal("expected non-nil items slice")
	}

	apiVersion := resp.Header().Get("Zotero-API-Version")
	if apiVersion != "3" {
		t.Errorf("expected Zotero-API-Version '3', got '%s'", apiVersion)
	}

	totalResultsStr := resp.Header().Get("Total-Results")
	if totalResultsStr == "" {
		t.Error("expected Total-Results header in response")
	}
}

func TestCloudApi_ReadAPITESTCollections(t *testing.T) {
	zot, group := getCloudTestClient(t)

	colls, resp, err := zot.GetCollectionsQuery(testCtx, group.Id, map[string]string{"limit": "10"})
	if err != nil {
		t.Fatalf("failed to query cloud collections for group %d: %v", group.Id, err)
	}
	if colls == nil {
		t.Fatal("expected non-nil collections slice")
	}

	apiVersion := resp.Header().Get("Zotero-API-Version")
	if apiVersion != "3" {
		t.Errorf("expected Zotero-API-Version '3', got '%s'", apiVersion)
	}

	totalResultsStr := resp.Header().Get("Total-Results")
	if totalResultsStr == "" {
		t.Error("expected Total-Results header in response")
	}
}

func TestCloudApi_ReadAPITESTTags(t *testing.T) {
	zot, group := getCloudTestClient(t)

	tags, lastModVer, err := zot.GetTags(testCtx, group.Id, 0)
	if err != nil {
		t.Fatalf("failed to query cloud tags for group %d: %v", group.Id, err)
	}
	if tags == nil {
		t.Fatal("expected non-nil tags slice")
	}
	if lastModVer <= 0 {
		t.Logf("last modified version for tags: %d", lastModVer)
	}
}

func TestCloudApi_PaginationAndFilters(t *testing.T) {
	zot, group := getCloudTestClient(t)

	_, resp, err := zot.GetItemsQuery(testCtx, group.Id, map[string]string{
		"start": "0",
		"limit": "5",
	})
	if err != nil {
		t.Fatalf("failed to query cloud items with pagination: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode())
	}

	totalResults := resp.Header().Get("Total-Results")
	if totalResults == "" {
		t.Error("expected Total-Results header for paginated query")
	}
}

func TestCloudApi_CreateAndRetainItems(t *testing.T) {
	zot, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	itemKey := model.CreateKey()
	initialTitle := "APITEST Cloud Retained Book: The Analytical Engine " + itemKey
	updatedTitle := "APITEST Cloud Retained Book: The Analytical Engine (Updated) " + itemKey

	itemData := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:      itemKey,
			ItemType: "book",
			Tags: []model.ItemTag{
				{Tag: "apitest-retained"},
				{Tag: "cloud-api-test"},
			},
			Creators: []model.ItemDataPerson{
				{
					CreatorType: "author",
					FirstName:   "Ada",
					LastName:    "Lovelace",
				},
			},
		},
		Title:        initialTitle,
		AbstractNote: "Sample cloud retained book entry created by automated tests for Zotero Cloud API inspection.",
	}
	itemData.SetString("ISBN", "978-0-123456-47-2")

	// 1. Create item in Cloud APITEST
	_, vResp, vErr := zot.GetItemsQuery(testCtx, group.Id, map[string]string{"limit": "1"})
	var lastModifiedVersion int64 = 0
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}
	createItemRes, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{itemData}, &lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") {
			t.Skipf("Cloud Zotero API endpoint does not allow write operations (read-only or authorization required): %v", err)
			return
		}
		t.Fatalf("failed to create item in Cloud APITEST: %v", err)
	}
	actualItemKey, err := createItemRes.CheckSuccess(0)
	if err != nil {
		t.Fatalf("failed to get created item key: %v", err)
	}

	// 2. Read item back from Cloud APITEST
	createdItem, err := zot.GetItemByKey(testCtx, group.Id, actualItemKey)
	if err != nil {
		t.Fatalf("failed to fetch created item %s: %v", actualItemKey, err)
	}
	if createdItem == nil {
		t.Fatalf("expected created item %s to exist, but got nil", actualItemKey)
	}
	if createdItem.Data.Title != initialTitle {
		t.Errorf("expected title '%s', got '%s'", initialTitle, createdItem.Data.Title)
	}
	if createdItem.Version <= 0 {
		t.Errorf("expected positive version after item creation, got %d", createdItem.Version)
	}

	// 3. Update item title in Cloud APITEST
	itemData.Title = updatedTitle
	itemData.Key = actualItemKey
	itemData.Version = createdItem.Version
	_, err = zot.UpdateItem(testCtx, group.Id, &itemData, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update item %s: %v", actualItemKey, err)
	}

	// 4. Verify updated item persists with new title
	updatedItem, err := zot.GetItemByKey(testCtx, group.Id, actualItemKey)
	if err != nil {
		t.Fatalf("failed to fetch updated item %s: %v", actualItemKey, err)
	}
	if updatedItem == nil {
		t.Fatalf("expected updated item %s to exist, but got nil", actualItemKey)
	}
	if updatedItem.Data.Title != updatedTitle {
		t.Errorf("expected updated title '%s', got '%s'", updatedTitle, updatedItem.Data.Title)
	}

	t.Logf("Successfully created and retained item '%s' (Key: %s, Version: %d) in Cloud APITEST", updatedTitle, actualItemKey, updatedItem.Version)
}

func TestCloudApi_CreateAndRetainCollections(t *testing.T) {
	zot, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	collKey := model.CreateKey()
	initialName := "APITEST Cloud Retained Subcollection " + collKey
	updatedName := "APITEST Cloud Retained Subcollection (Renamed) " + collKey

	collData := model.CollectionData{
		Key:  collKey,
		Name: initialName,
	}

	// 1. Create collection in Cloud APITEST
	_, vResp, vErr := zot.GetCollectionsQuery(testCtx, group.Id, map[string]string{"limit": "1"})
	var lastModifiedVersion int64 = 0
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}
	actualCollKey, err := zot.UpdateCollection(testCtx, group.Id, &collData, &lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") {
			t.Skipf("Cloud Zotero API endpoint does not allow collection write operations (read-only or authorization required): %v", err)
			return
		}
		t.Fatalf("failed to create collection in Cloud APITEST: %v", err)
	}

	// 2. Read collection back from Cloud APITEST
	createdColl, err := zot.GetCollectionByKey(testCtx, group.Id, actualCollKey)
	if err != nil {
		t.Fatalf("failed to fetch created collection %s: %v", actualCollKey, err)
	}
	if createdColl == nil {
		t.Fatalf("expected created collection %s to exist, but got nil", actualCollKey)
	}
	if createdColl.Data.Name != initialName {
		t.Errorf("expected collection name '%s', got '%s'", initialName, createdColl.Data.Name)
	}
	if createdColl.Version <= 0 {
		t.Errorf("expected positive version after collection creation, got %d", createdColl.Version)
	}

	// 3. Update collection name in Cloud APITEST
	createdColl.Data.Name = updatedName
	_, err = zot.UpdateCollection(testCtx, group.Id, &createdColl.Data, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update collection %s: %v", actualCollKey, err)
	}

	// 4. Verify updated collection persists with new name
	updatedColl, err := zot.GetCollectionByKey(testCtx, group.Id, actualCollKey)
	if err != nil {
		t.Fatalf("failed to fetch updated collection %s: %v", actualCollKey, err)
	}
	if updatedColl == nil {
		t.Fatalf("expected updated collection %s to exist, but got nil", actualCollKey)
	}
	if updatedColl.Data.Name != updatedName {
		t.Errorf("expected updated collection name '%s', got '%s'", updatedName, updatedColl.Data.Name)
	}

	t.Logf("Successfully created and retained subcollection '%s' (Key: %s, Version: %d) in Cloud APITEST", updatedName, actualCollKey, updatedColl.Version)
}

func TestCloudApi_CreateAndRetainAttachment(t *testing.T) {
	zot, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	// 1. Create parent book item in Cloud APITEST
	parentKey := model.CreateKey()
	parentTitle := "APITEST Cloud Book with Attachment " + parentKey
	parentItem := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:      parentKey,
			ItemType: "book",
			Tags: []model.ItemTag{
				{Tag: "apitest-retained"},
				{Tag: "attachment-parent"},
			},
			Creators: []model.ItemDataPerson{
				{
					CreatorType: "author",
					FirstName:   "Charles",
					LastName:    "Babbage",
				},
			},
		},
		Title:        parentTitle,
		AbstractNote: "Parent book entry created to attach binary document in APITEST.",
	}

	var lastModifiedVersion int64 = 0
	_, vResp, vErr := zot.GetItemsQuery(testCtx, group.Id, map[string]string{"limit": "1"})
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}

	parentCreateRes, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{parentItem}, &lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") {
			t.Skipf("Cloud Zotero API endpoint does not allow write operations: %v", err)
			return
		}
		t.Fatalf("failed to create parent book item: %v", err)
	}
	actualParentKey, err := parentCreateRes.CheckSuccess(0)
	if err != nil {
		t.Fatalf("failed to get parent item key: %v", err)
	}

	// 2. Prepare attachment binary data
	attKey := model.CreateKey()
	rawBinaryPayload := []byte("%PDF-1.4 sample binary file content for zsync cloud test attachment " + attKey)
	expectedMD5 := fmt.Sprintf("%x", md5.Sum(rawBinaryPayload))

	// 3. Create child attachment item
	attItem := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:        attKey,
			ItemType:   "attachment",
			ParentItem: actualParentKey,
			Tags: []model.ItemTag{
				{Tag: "apitest-retained"},
				{Tag: "attachment-child"},
			},
		},
		Title:       "Analytical Engine Diagram " + attKey,
		LinkMode:    "imported_file",
		ContentType: "application/pdf",
		Filename:    "diagram.pdf",
	}

	attCreateRes, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{attItem}, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to create child attachment item: %v", err)
	}
	actualAttKey, err := attCreateRes.CheckSuccess(0)
	if err != nil {
		t.Fatalf("failed to get child attachment item key: %v", err)
	}

	// 4. Upload binary attachment data via 3-step upload API
	mtime := time.Now().UnixMilli()
	uploadedMD5, err := zot.UploadAttachment(testCtx, group.Id, actualAttKey, rawBinaryPayload, "diagram.pdf", "application/pdf", mtime, expectedMD5)
	if err != nil {
		t.Fatalf("UploadAttachment failed: %v", err)
	}
	if uploadedMD5 != expectedMD5 {
		t.Errorf("expected uploaded MD5 '%s', got '%s'", expectedMD5, uploadedMD5)
	}

	// 5. Download binary attachment data and verify content and MD5
	downloadedBytes, contentType, downloadedMD5, err := zot.DownloadAttachment(testCtx, group.Id, actualAttKey)
	if err != nil {
		t.Fatalf("DownloadAttachment failed: %v", err)
	}
	if !bytes.Equal(downloadedBytes, rawBinaryPayload) {
		t.Errorf("downloaded content mismatch: got %d bytes, expected %d bytes", len(downloadedBytes), len(rawBinaryPayload))
	}
	if downloadedMD5 != expectedMD5 {
		t.Errorf("expected downloaded MD5 '%s', got '%s'", expectedMD5, downloadedMD5)
	}
	if !strings.Contains(contentType, "pdf") && !strings.Contains(contentType, "octet-stream") {
		t.Logf("downloaded attachment Content-Type: %s", contentType)
	}

	t.Logf("Successfully created and verified attachment item '%s' (Key: %s, MD5: %s) in Cloud APITEST", attItem.Title, actualAttKey, downloadedMD5)
}

func TestCloudApi_VerifyRetainedData(t *testing.T) {
	zot, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	// 1. Query items in APITEST to verify retained items are accessible
	items, resp, err := zot.GetItemsQuery(testCtx, group.Id, map[string]string{
		"limit": "25",
	})
	if err != nil {
		t.Fatalf("failed to query items for verification in APITEST: %v", err)
	}
	if items == nil {
		t.Fatal("expected non-nil items list during retention verification")
	}

	totalResultsStr := resp.Header().Get("Total-Results")
	totalResults, _ := strconv.ParseInt(totalResultsStr, 10, 64)
	t.Logf("Verification: APITEST group contains %d items (Total-Results: %d, Page Count: %d)", totalResults, totalResults, len(items))

	// 2. Query collections in APITEST to verify retained collections are accessible
	colls, collResp, err := zot.GetCollectionsQuery(testCtx, group.Id, map[string]string{
		"limit": "25",
	})
	if err != nil {
		t.Fatalf("failed to query collections for verification in APITEST: %v", err)
	}
	if colls == nil {
		t.Fatal("expected non-nil collections list during retention verification")
	}

	totalCollsStr := collResp.Header().Get("Total-Results")
	totalColls, _ := strconv.ParseInt(totalCollsStr, 10, 64)
	t.Logf("Verification: APITEST group contains %d collections (Total-Results: %d, Page Count: %d)", totalColls, totalColls, len(colls))

	for _, it := range items {
		t.Logf("  - Retained Item: Key=%s Type=%s Title='%s' Version=%d", it.Key, it.Data.ItemType, it.Data.Title, it.Version)
	}
	for _, cl := range colls {
		t.Logf("  - Retained Collection: Key=%s Name='%s' Version=%d", cl.Key, cl.Data.Name, cl.Version)
	}
}

func TestCloudApi_MockServerFullCycle(t *testing.T) {
	itemsStore := make(map[string]model.Item)
	collectionsStore := make(map[string]model.Collection)
	var currentVersion int64 = 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "zsync") {
			t.Errorf("expected User-Agent starting with 'zsync', got '%s'", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Zotero-API-Version", "3")
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/groups/6642571":
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, model.Group{
				Id:      6642571,
				Version: currentVersion,
				Data: model.GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/groups/6642571/items":
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.Header().Set("Total-Results", strconv.Itoa(len(itemsStore)))
			itemsList := make([]model.Item, 0, len(itemsStore))
			for _, it := range itemsStore {
				itemsList = append(itemsList, it)
			}
			json.MarshalWrite(w, itemsList)

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/items":
			var posted []model.ItemGeneric
			if err := json.UnmarshalRead(r.Body, &posted); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result := model.ItemCollectionCreateResult{
				Success:    make(map[string]string),
				Successful: make(map[string]model.Item),
				Unchanged:  make(map[string]string),
				Failed:     make(map[string]model.ItemCollectionCreateResultFailed),
			}
			currentVersion++
			for idx, itemData := range posted {
				key := itemData.Key
				if key == "" {
					key = model.CreateKey()
					itemData.Key = key
				}
				item := model.Item{
					Key:     key,
					Version: currentVersion,
					Data:    itemData,
				}
				itemsStore[key] = item
				idxStr := strconv.Itoa(idx)
				result.Success[idxStr] = key
				result.Successful[idxStr] = item
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, result)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/groups/6642571/items/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/items/")
			item, found := itemsStore[key]
			if !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, item)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/groups/6642571/items/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/items/")
			delete(itemsStore, key)
			currentVersion++
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && r.URL.Path == "/groups/6642571/collections":
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.Header().Set("Total-Results", strconv.Itoa(len(collectionsStore)))
			collsList := make([]model.Collection, 0, len(collectionsStore))
			for _, cl := range collectionsStore {
				collsList = append(collsList, cl)
			}
			json.MarshalWrite(w, collsList)

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/collections":
			var posted []model.CollectionData
			if err := json.UnmarshalRead(r.Body, &posted); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result := model.ItemCollectionCreateResult{
				Success:    make(map[string]string),
				Successful: make(map[string]model.Item),
				Unchanged:  make(map[string]string),
				Failed:     make(map[string]model.ItemCollectionCreateResultFailed),
			}
			currentVersion++
			for idx, collData := range posted {
				key := collData.Key
				if key == "" {
					key = model.CreateKey()
					collData.Key = key
				}
				coll := model.Collection{
					Key:     key,
					Version: currentVersion,
					Data:    collData,
				}
				collectionsStore[key] = coll
				idxStr := strconv.Itoa(idx)
				result.Success[idxStr] = key
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, result)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/groups/6642571/collections/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/collections/")
			coll, found := collectionsStore[key]
			if !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, coll)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/groups/6642571/collections/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/collections/")
			delete(collectionsStore, key)
			currentVersion++
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewClient(testCtx, server.URL, "TEST_API_KEY", &logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	group, err := zot.GetGroup(testCtx, 6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	// 1. Items Full Cycle
	itemKey := model.CreateKey()
	itemData := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:      itemKey,
			ItemType: "book",
		},
		Title: "Mock Cloud Book",
	}

	var lastMod int64 = 1
	cRes, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{itemData}, &lastMod)
	if err != nil {
		t.Fatalf("CreateItems failed: %v", err)
	}
	if len(cRes.Success) != 1 {
		t.Fatalf("expected 1 created item, got %d", len(cRes.Success))
	}

	fetchedItem, err := zot.GetItemByKey(testCtx, group.Id, itemKey)
	if err != nil {
		t.Fatalf("GetItemByKey failed: %v", err)
	}
	if fetchedItem == nil || fetchedItem.Data.Title != "Mock Cloud Book" {
		t.Errorf("unexpected fetched item: %v", fetchedItem)
	}

	itemData.Title = "Mock Cloud Book (Updated)"
	itemData.Version = fetchedItem.Version
	_, err = zot.UpdateItem(testCtx, group.Id, &itemData, &lastMod)
	if err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}

	updatedItem, err := zot.GetItemByKey(testCtx, group.Id, itemKey)
	if err != nil {
		t.Fatalf("GetItemByKey updated failed: %v", err)
	}
	if updatedItem == nil || updatedItem.Data.Title != "Mock Cloud Book (Updated)" {
		t.Errorf("unexpected updated item: %v", updatedItem)
	}

	err = zot.DeleteItem(testCtx, group.Id, itemKey, lastMod)
	if err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}

	deletedItem, err := zot.GetItemByKey(testCtx, group.Id, itemKey)
	if err != nil {
		t.Fatalf("unexpected error getting deleted item: %v", err)
	}
	if deletedItem != nil {
		t.Errorf("expected nil for deleted item, got %v", deletedItem)
	}

	// 2. Collections Full Cycle
	collKey := model.CreateKey()
	collData := model.CollectionData{
		Key:  collKey,
		Name: "Mock Cloud Collection",
	}

	collRes, err := zot.CreateCollections(testCtx, group.Id, []model.CollectionData{collData})
	if err != nil {
		t.Fatalf("CreateCollections failed: %v", err)
	}
	if len(collRes.Success) != 1 {
		t.Fatalf("expected 1 created collection, got %d", len(collRes.Success))
	}

	fetchedColl, err := zot.GetCollectionByKey(testCtx, group.Id, collKey)
	if err != nil {
		t.Fatalf("GetCollectionByKey failed: %v", err)
	}
	if fetchedColl == nil || fetchedColl.Data.Name != "Mock Cloud Collection" {
		t.Errorf("unexpected fetched collection: %v", fetchedColl)
	}

	collData.Name = "Mock Cloud Collection (Updated)"
	collData.Version = fetchedColl.Version
	_, err = zot.UpdateCollection(testCtx, group.Id, &collData, &lastMod)
	if err != nil {
		t.Fatalf("UpdateCollection failed: %v", err)
	}

	updatedColl, err := zot.GetCollectionByKey(testCtx, group.Id, collKey)
	if err != nil {
		t.Fatalf("GetCollectionByKey updated failed: %v", err)
	}
	if updatedColl == nil || updatedColl.Data.Name != "Mock Cloud Collection (Updated)" {
		t.Errorf("unexpected updated collection: %v", updatedColl)
	}

	err = zot.DeleteCollection(testCtx, group.Id, collKey, lastMod)
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}

	deletedColl, err := zot.GetCollectionByKey(testCtx, group.Id, collKey)
	if err != nil {
		t.Fatalf("unexpected error getting deleted collection: %v", err)
	}
	if deletedColl != nil {
		t.Errorf("expected nil for deleted collection, got %v", deletedColl)
	}
}

func TestCloudApi_RetryAfterAndBackoff_MockServer(t *testing.T) {
	var probeCalls int
	var getGroupCalls int
	var postItemCalls int
	var deleteItemCalls int
	var postCollCalls int
	var deleteCollCalls int
	var currentVersion int64 = 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "zsync") {
			t.Errorf("expected User-Agent starting with 'zsync', got '%s'", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Zotero-API-Version", "3")
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/groups/6642571/items" && r.URL.Query().Get("limit") == "1":
			probeCalls++
			if probeCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			w.Header().Set("Backoff", "1")
			w.Header().Set("Total-Results", "0")
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`))

		case r.Method == http.MethodGet && r.URL.Path == "/groups/6642571":
			getGroupCalls++
			if getGroupCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusOK)
			json.MarshalWrite(w, model.Group{
				Id:      6642571,
				Version: currentVersion,
				Data: model.GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/items":
			postItemCalls++
			if postItemCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			currentVersion++
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusOK)
			result := model.ItemCollectionCreateResult{
				Success:    map[string]string{"0": "ITEM123"},
				Successful: map[string]model.Item{"0": {Key: "ITEM123", Version: currentVersion}},
				Unchanged:  map[string]string{},
				Failed:     map[string]model.ItemCollectionCreateResultFailed{},
			}
			json.MarshalWrite(w, result)

		case r.Method == http.MethodDelete && r.URL.Path == "/groups/6642571/items/ITEM123":
			deleteItemCalls++
			if deleteItemCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/collections":
			postCollCalls++
			if postCollCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			currentVersion++
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusOK)
			result := model.ItemCollectionCreateResult{
				Success:    map[string]string{"0": "COLL123"},
				Successful: map[string]model.Item{},
				Unchanged:  map[string]string{},
				Failed:     map[string]model.ItemCollectionCreateResultFailed{},
			}
			json.MarshalWrite(w, result)

		case r.Method == http.MethodDelete && r.URL.Path == "/groups/6642571/collections/COLL123":
			deleteCollCalls++
			if deleteCollCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewClient(testCtx, server.URL, "TEST_API_KEY", &logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// 1. Verify checkCloudZoteroAvailable handles 429 Retry-After and Backoff
	checkCloudZoteroAvailable(t, server.URL, 6642571, "TEST_API_KEY")
	if probeCalls < 2 {
		t.Errorf("expected at least 2 probe calls with retry, got %d", probeCalls)
	}

	// 2. Verify GetGroup retries on 429
	group, err := zot.GetGroup(testCtx, 6642571)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}
	if getGroupCalls < 2 {
		t.Errorf("expected at least 2 getGroupCalls with retry, got %d", getGroupCalls)
	}

	// 3. Verify CreateItems and DeleteItem retry on 429
	itemData := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:      "ITEM123",
			ItemType: "book",
		},
		Title: "Test Retry Book",
	}
	var lastMod int64 = 1
	_, err = zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{itemData}, &lastMod)
	if err != nil {
		t.Fatalf("CreateItems failed: %v", err)
	}
	if postItemCalls < 2 {
		t.Errorf("expected at least 2 postItemCalls with retry, got %d", postItemCalls)
	}

	err = zot.DeleteItem(testCtx, group.Id, "ITEM123", lastMod)
	if err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}
	if deleteItemCalls < 2 {
		t.Errorf("expected at least 2 deleteItemCalls with retry, got %d", deleteItemCalls)
	}

	// 4. Verify CreateCollections and DeleteCollection retry on 429
	collData := model.CollectionData{
		Key:  "COLL123",
		Name: "Test Retry Collection",
	}
	_, err = zot.CreateCollections(testCtx, group.Id, []model.CollectionData{collData})
	if err != nil {
		t.Fatalf("CreateCollections failed: %v", err)
	}
	if postCollCalls < 2 {
		t.Errorf("expected at least 2 postCollCalls with retry, got %d", postCollCalls)
	}

	err = zot.DeleteCollection(testCtx, group.Id, "COLL123", lastMod)
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}
	if deleteCollCalls < 2 {
		t.Errorf("expected at least 2 deleteCollCalls with retry, got %d", deleteCollCalls)
	}
}

func TestCloudApi_AttachmentUploadAndDownload_MockServer(t *testing.T) {
	uploadedStorageFiles := make(map[string][]byte)
	itemsStore := make(map[string]model.Item)
	var currentVersion int64 = 1

	var serverURL string
	var authAttempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "zsync") {
			t.Errorf("expected User-Agent starting with 'zsync', got '%s'", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Zotero-API-Version", "3")

		// 1. Group info: GET /groups/6642571
		if r.Method == http.MethodGet && r.URL.Path == "/groups/6642571" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, model.Group{
				Id:      6642571,
				Version: currentVersion,
				Data: model.GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})
			return
		}

		// 2. Query items: GET /groups/6642571/items
		if r.Method == http.MethodGet && r.URL.Path == "/groups/6642571/items" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.Header().Set("Total-Results", strconv.Itoa(len(itemsStore)))
			itemsList := make([]model.Item, 0, len(itemsStore))
			for _, it := range itemsStore {
				itemsList = append(itemsList, it)
			}
			json.MarshalWrite(w, itemsList)
			return
		}

		// 3. Create items: POST /groups/6642571/items
		if r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/items" {
			w.Header().Set("Content-Type", "application/json")
			var rawItems []model.ItemGeneric
			if err := json.UnmarshalRead(r.Body, &rawItems); err != nil {
				t.Fatalf("failed to decode create items payload: %v", err)
			}
			currentVersion++
			respMap := map[string]any{
				"success":    map[string]string{},
				"unchanged":  map[string]string{},
				"failed":     map[string]any{},
				"successful": map[string]any{},
			}
			for i, raw := range rawItems {
				k := raw.Key
				if k == "" {
					k = fmt.Sprintf("KEY%04d", len(itemsStore)+1)
				}
				raw.Key = k
				raw.Version = currentVersion
				item := model.Item{
					Key:     k,
					Version: currentVersion,
					Data:    raw,
				}
				itemsStore[k] = item
				idxStr := strconv.Itoa(i)
				respMap["success"].(map[string]string)[idxStr] = k
				respMap["successful"].(map[string]any)[idxStr] = map[string]any{
					"key":     k,
					"version": currentVersion,
					"data":    raw,
				}
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.MarshalWrite(w, respMap)
			return
		}

		// 4. Attachment File endpoints: POST /groups/6642571/items/{key}/file and GET /groups/6642571/items/{key}/file
		if strings.HasPrefix(r.URL.Path, "/groups/6642571/items/") && strings.HasSuffix(r.URL.Path, "/file") {
			parts := strings.Split(r.URL.Path, "/")
			itemKey := parts[4]

			// File download: GET /groups/6642571/items/{key}/file
			if r.Method == http.MethodGet {
				fileData, ok := uploadedStorageFiles[itemKey]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				md5Sum := fmt.Sprintf("%x", md5.Sum(fileData))
				w.Header().Set("Content-Type", "application/pdf")
				w.Header().Set("ETag", fmt.Sprintf("\"%s\"", md5Sum))
				w.WriteHeader(http.StatusOK)
				w.Write(fileData)
				return
			}

			// File upload auth / register: POST /groups/6642571/items/{key}/file
			if r.Method == http.MethodPost {
				if err := r.ParseForm(); err != nil {
					t.Fatalf("failed to parse form in file endpoint: %v", err)
				}

				// Step 3: Register upload
				if uploadKey := r.FormValue("upload"); uploadKey != "" {
					expectedKey := "UPKEY_" + itemKey
					if uploadKey != expectedKey {
						t.Errorf("expected upload key '%s', got '%s'", expectedKey, uploadKey)
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}

				// Step 1: Upload authorization request
				fileMd5 := r.FormValue("md5")
				if fileMd5 == "" {
					t.Error("expected md5 in upload auth form")
				}

				targetItem := itemsStore[itemKey]

				// Test retry on first attempt for retry item
				if strings.Contains(targetItem.Data.Title, "Retry") && authAttempts == 0 {
					authAttempts++
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}

				// Test exists == 1 case
				if strings.Contains(targetItem.Data.Title, "Existing") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.MarshalWrite(w, map[string]string{
						"exists": "1",
					})
					return
				}

				// Standard upload authorization response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.MarshalWrite(w, map[string]string{
					"url":         fmt.Sprintf("%s/storage/upload/%s", serverURL, itemKey),
					"contentType": "multipart/form-data; boundary=---boundary",
					"prefix":      "---prefix\r\n",
					"suffix":      "\r\n---suffix",
					"uploadKey":   "UPKEY_" + itemKey,
				})
				return
			}
		}

		// 5. Mock Storage upload target: POST /storage/upload/{key}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/upload/") {
			itemKey := strings.TrimPrefix(r.URL.Path, "/storage/upload/")
			bodyBuf := new(bytes.Buffer)
			if _, err := bodyBuf.ReadFrom(r.Body); err != nil {
				t.Fatalf("failed to read storage upload body: %v", err)
			}
			rawPayload := bodyBuf.Bytes()

			prefix := "---prefix\r\n"
			suffix := "\r\n---suffix"
			if !bytes.HasPrefix(rawPayload, []byte(prefix)) || !bytes.HasSuffix(rawPayload, []byte(suffix)) {
				t.Errorf("storage payload missing expected prefix/suffix formatting")
			}

			// Extract inner payload
			inner := rawPayload[len(prefix) : len(rawPayload)-len(suffix)]
			uploadedStorageFiles[itemKey] = inner

			w.WriteHeader(http.StatusCreated)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL = server.URL

	clientLogger := zerolog.Nop()
	zot, err := NewClient(testCtx, server.URL, "TEST_API_KEY", &clientLogger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	group, err := zot.GetGroup(testCtx, 6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	// 1. Create parent item
	parentKey := "PARENT_001"
	parentItem := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:      parentKey,
			ItemType: "book",
		},
		Title: "Mock Parent Book For Attachment",
	}

	var lastMod int64 = 1
	if _, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{parentItem}, &lastMod); err != nil {
		t.Fatalf("failed to create parent item on mock server: %v", err)
	}

	// 2. Prepare attachment binary data
	attKey := "ATT_NEW_001"
	rawBinaryPayload := []byte("%PDF-1.4 sample binary file content for zsync mock test attachment")
	expectedMD5 := fmt.Sprintf("%x", md5.Sum(rawBinaryPayload))

	// 3. Create child attachment item and upload
	attItem := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:        attKey,
			ItemType:   "attachment",
			ParentItem: parentKey,
		},
		Title:       "Research Attachment Document.pdf",
		LinkMode:    "imported_file",
		ContentType: "application/pdf",
		Filename:    "document.pdf",
	}

	if _, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{attItem}, &lastMod); err != nil {
		t.Fatalf("failed to create attachment item: %v", err)
	}

	uploadedMD5, err := zot.UploadAttachment(testCtx, group.Id, attKey, rawBinaryPayload, "document.pdf", "application/pdf", 1620000000, expectedMD5)
	if err != nil {
		t.Fatalf("UploadAttachment failed: %v", err)
	}
	if uploadedMD5 != expectedMD5 {
		t.Errorf("expected uploaded MD5 '%s', got '%s'", expectedMD5, uploadedMD5)
	}

	// Verify storage endpoint received exact binary content
	storedBytes, ok := uploadedStorageFiles[attKey]
	if !ok {
		t.Fatalf("expected file '%s' to be stored in mock storage", attKey)
	}
	if !bytes.Equal(storedBytes, rawBinaryPayload) {
		t.Errorf("stored bytes do not match original payload: got %s, expected %s", string(storedBytes), string(rawBinaryPayload))
	}

	// 4. Download attachment
	dlData, contentType, dlMD5, err := zot.DownloadAttachment(testCtx, group.Id, attKey)
	if err != nil {
		t.Fatalf("DownloadAttachment failed: %v", err)
	}
	if dlMD5 != expectedMD5 {
		t.Errorf("expected downloaded MD5 '%s', got '%s'", expectedMD5, dlMD5)
	}
	if contentType != "application/pdf" {
		t.Errorf("expected Content-Type 'application/pdf', got '%s'", contentType)
	}
	if !bytes.Equal(dlData, rawBinaryPayload) {
		t.Errorf("downloaded bytes mismatch: got %d, expected %d", len(dlData), len(rawBinaryPayload))
	}

	// 5. Test upload with Retry-After on first auth attempt
	attKeyRetry := "ATT_RETRY_001"
	attItemRetry := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:        attKeyRetry,
			ItemType:   "attachment",
			ParentItem: parentKey,
		},
		Title:       "Retry Attachment Document.pdf",
		LinkMode:    "imported_file",
		ContentType: "application/pdf",
		Filename:    "retry.pdf",
	}
	if _, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{attItemRetry}, &lastMod); err != nil {
		t.Fatalf("failed to create retry attachment item: %v", err)
	}
	retryPayload := []byte("retry payload bytes")
	retryMD5 := fmt.Sprintf("%x", md5.Sum(retryPayload))
	upRetryMD5, err := zot.UploadAttachment(testCtx, group.Id, attKeyRetry, retryPayload, "retry.pdf", "application/pdf", 1620000000, retryMD5)
	if err != nil {
		t.Fatalf("UploadAttachment with retry failed: %v", err)
	}
	if upRetryMD5 != retryMD5 {
		t.Errorf("expected retry MD5 '%s', got '%s'", retryMD5, upRetryMD5)
	}
	if authAttempts < 1 {
		t.Errorf("expected at least 1 auth attempt retry, got %d", authAttempts)
	}

	// 6. Test upload with exists == 1
	attKeyExists := "ATT_EXISTS_001"
	attItemExists := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:        attKeyExists,
			ItemType:   "attachment",
			ParentItem: parentKey,
		},
		Title:       "Existing Attachment Document.pdf",
		LinkMode:    "imported_file",
		ContentType: "application/pdf",
		Filename:    "exists.pdf",
	}
	if _, err := zot.CreateItems(testCtx, group.Id, []model.ItemGeneric{attItemExists}, &lastMod); err != nil {
		t.Fatalf("failed to create exists attachment item: %v", err)
	}
	existsPayload := []byte("exists payload bytes")
	existsMD5 := fmt.Sprintf("%x", md5.Sum(existsPayload))
	upExistsMD5, err := zot.UploadAttachment(testCtx, group.Id, attKeyExists, existsPayload, "exists.pdf", "application/pdf", 1620000000, existsMD5)
	if err != nil {
		t.Fatalf("UploadAttachment with exists failed: %v", err)
	}
	if upExistsMD5 != existsMD5 {
		t.Errorf("expected exists MD5 '%s', got '%s'", existsMD5, upExistsMD5)
	}
}
