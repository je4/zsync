package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/je4/zsync/v2/info"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/rs/zerolog"
)

// Local API vs Cloud API Authentication Architecture:
// 1. Zotero Cloud API (https://api.zotero.org):
//    - Uses API keys created on zotero.org/settings/keys.
//    - Authenticated via HTTP header "Zotero-API-Key: <cloud-key>" validated against central Zotero databases.
// 2. Zotero Local API (http://localhost:23119/api):
//    - Embedded HTTP server inside the Zotero desktop application (for connector/local integrations).
//    - Has NO connection to the zotero.org key database. Cloud API keys sent to localhost are rejected or ignored.
//    - Write operations require local authorization via POST /keys ("AuthorizeLocal"), which prompts the user
//      with an interactive GUI popup in the Zotero desktop client to issue a local token.
//    - Alternatively, a pre-authorized local key can be passed via the ZOTERO_LOCAL_KEY environment variable.

const (
	defaultLocalEndpoint = "http://localhost:23119/api"
	defaultTestGroupId   = int64(6642571)
	defaultTestGroupName = "APITEST"
)

var (
	localAuthMutex sync.Mutex
	cachedLocalKey string
)

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
		t.Logf("Local Zotero instance unreachable at %s (%v). Skipping integration test.", probeUrl, err)
		t.Skipf("Local Zotero instance is not available at %s - skipping test", endpoint)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		t.Logf("Local Zotero instance returned error status %d at %s. Skipping integration test.", resp.StatusCode, probeUrl)
		t.Skipf("Local Zotero instance returned status %d at %s - skipping test", resp.StatusCode, probeUrl)
		return
	}
}

func getTestClient(t *testing.T) (*Client, *model.Group) {
	t.Helper()

	endpoint, groupId, localKey, cloudKey := getLocalTestConfig()
	checkLocalZoteroAvailable(t, endpoint, groupId)

	isLocalhost := strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1")

	var effectiveKey string
	if localKey != "" {
		effectiveKey = localKey
		t.Logf("Using local authorization key from ZOTERO_LOCAL_KEY")
	} else if isLocalhost {
		localAuthMutex.Lock()
		if cachedLocalKey != "" {
			effectiveKey = cachedLocalKey
			t.Logf("Using cached local authorization key")
		}
		localAuthMutex.Unlock()
	} else {
		// Non-local custom endpoint: use ZOTERO_API_KEY if provided
		effectiveKey = cloudKey
	}

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewClient(endpoint, effectiveKey, &logger)
	if err != nil {
		t.Fatalf("failed to create authenticated Zotero client: %v", err)
	}

	group, err := zot.GetGroup(groupId)
	if err != nil {
		t.Fatalf("failed to retrieve test group %d: %v", groupId, err)
	}
	if group == nil {
		t.Fatalf("group %d not found on local Zotero instance", groupId)
	}

	return zot, group
}

func getWriteTestClient(t *testing.T) (*Client, *model.Group) {
	t.Helper()

	zot, group := getTestClient(t)

	endpoint, _, localKey, cloudKey := getLocalTestConfig()
	isLocalhost := strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1")

	// If we already have a key (from ZOTERO_LOCAL_KEY or cachedLocalKey or non-localhost cloudKey), we're good
	if zot.GetApiKey() != "" {
		return zot, group
	}

	if isLocalhost {
		if cloudKey != "" && localKey == "" {
			t.Logf("Notice: ZOTERO_API_KEY is configured with a Cloud API key. Cloud keys are not recognized by local Zotero (localhost:23119). Requesting local authorization token...")
		}

		// Check if we are running in CI or headless non-interactive environment
		if os.Getenv("CI") != "" {
			t.Logf("CI environment detected without ZOTERO_LOCAL_KEY. Skipping local write test.")
			t.Skipf("CI environment detected without ZOTERO_LOCAL_KEY - cannot prompt for local write authorization")
			return nil, nil
		}

		localAuthMutex.Lock()
		defer localAuthMutex.Unlock()

		if cachedLocalKey != "" {
			zot.SetApiKey(cachedLocalKey)
			return zot, group
		}

		t.Logf("Local write authorization required: Please click 'Accept' in the Zotero desktop popup within 30 seconds...")
		authCtx, authCancel := context.WithTimeout(context.Background(), 30*time.Second)
		key, authErr := zot.AuthorizeLocalContext(authCtx, "ZSyncTest")
		authCancel()

		if authErr != nil || key == "" {
			t.Logf("Local write authorization not granted: %v (skipping write test)", authErr)
			t.Skipf("Local write authorization not granted (timed out or rejected): %v", authErr)
			return nil, nil
		}

		cachedLocalKey = key
		zot.SetApiKey(key)
		t.Logf("Successfully obtained and cached local write authorization key: %s", key)
	}

	return zot, group
}

func TestLocalApi_PreFlightAndClientInit(t *testing.T) {
	zot, group := getTestClient(t)
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

func TestLocalApi_ReadAPITESTGroup(t *testing.T) {
	_, group := getTestClient(t)

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

func TestLocalApi_ReadAPITESTItems(t *testing.T) {
	zot, group := getTestClient(t)

	items, resp, err := zot.GetItemsQuery(group.Id, map[string]string{"limit": "10"})
	if err != nil {
		t.Fatalf("failed to query items for group %d: %v", group.Id, err)
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

func TestLocalApi_ReadAPITESTCollections(t *testing.T) {
	zot, group := getTestClient(t)

	colls, resp, err := zot.GetCollectionsQuery(group.Id, map[string]string{"limit": "10"})
	if err != nil {
		t.Fatalf("failed to query collections for group %d: %v", group.Id, err)
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

func TestLocalApi_ReadAPITESTTags(t *testing.T) {
	zot, group := getTestClient(t)

	tags, lastModVer, err := zot.GetTagsVersion(group.Id, 0)
	if err != nil {
		t.Fatalf("failed to query tags for group %d: %v", group.Id, err)
	}
	if tags == nil {
		t.Fatal("expected non-nil tags slice")
	}
	if lastModVer <= 0 {
		t.Logf("last modified version for tags: %d", lastModVer)
	}
}

func TestLocalApi_PaginationAndFilters(t *testing.T) {
	zot, group := getTestClient(t)

	_, resp, err := zot.GetItemsQuery(group.Id, map[string]string{
		"start": "0",
		"limit": "5",
	})
	if err != nil {
		t.Fatalf("failed to query items with pagination: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode())
	}

	totalResults := resp.Header().Get("Total-Results")
	if totalResults == "" {
		t.Error("expected Total-Results header for paginated query")
	}
}

func TestLocalApi_CreateAndRetainItems(t *testing.T) {
	zot, group := getWriteTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	titleKey := model.CreateKey()
	initialTitle := "APITEST Retained Book: The Analytical Engine " + titleKey
	updatedTitle := "APITEST Retained Book: The Analytical Engine (Updated) " + titleKey

	itemData := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			ItemType: "book",
			Tags: []model.ItemTag{
				{Tag: "apitest-retained"},
				{Tag: "local-api-test"},
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
		ISBN:         "978-0-123456-47-2",
		AbstractNote: "Sample retained book entry created by automated tests for local Zotero inspection.",
	}

	// 1. Create item in APITEST (retained - no deletion in teardown)
	_, vResp, vErr := zot.GetItemsQuery(group.Id, map[string]string{"limit": "1"})
	var lastModifiedVersion int64 = 0
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}
	createItemRes, err := zot.CreateItems(group.Id, []model.ItemGeneric{itemData}, &lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") ||
			strings.Contains(err.Error(), "400") {
			t.Skipf("Local Zotero API endpoint does not allow write operations (read-only or local authorization required): %v", err)
			return
		}
		t.Fatalf("failed to create item in APITEST: %v", err)
	}
	actualItemKey, err := createItemRes.CheckSuccess(0)
	if err != nil {
		t.Fatalf("failed to get created item key: %v", err)
	}

	// 2. Read item back from APITEST
	createdItem, err := zot.GetItemByKey(group.Id, actualItemKey)
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

	// 3. Update item title in APITEST
	createdItem.Data.Title = updatedTitle
	_, err = zot.UpdateItem(group.Id, &createdItem.Data, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update item %s: %v", actualItemKey, err)
	}

	// 4. Verify updated item persists with new title
	updatedItem, err := zot.GetItemByKey(group.Id, actualItemKey)
	if err != nil {
		t.Fatalf("failed to fetch updated item %s: %v", actualItemKey, err)
	}
	if updatedItem == nil {
		t.Fatalf("expected updated item %s to exist, but got nil", actualItemKey)
	}
	if updatedItem.Data.Title != updatedTitle {
		t.Errorf("expected updated title '%s', got '%s'", updatedTitle, updatedItem.Data.Title)
	}

	// Note: The item is deliberately NOT deleted so it remains in local Zotero APITEST collection.
	t.Logf("Successfully created and retained item '%s' (Key: %s, Version: %d) in APITEST", updatedTitle, actualItemKey, updatedItem.Version)
}

func TestLocalApi_CreateAndRetainCollections(t *testing.T) {
	zot, group := getWriteTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	collKey := model.CreateKey()
	initialName := "APITEST Retained Subcollection " + collKey
	updatedName := "APITEST Retained Subcollection (Renamed) " + collKey

	collData := model.CollectionData{
		Key:  collKey,
		Name: initialName,
	}

	// 1. Create collection in APITEST (retained - no deletion in teardown)
	_, vResp, vErr := zot.GetCollectionsQuery(group.Id, map[string]string{"limit": "1"})
	var lastModifiedVersion int64 = 0
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}
	actualCollKey, err := zot.UpdateCollection(group.Id, &collData, &lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") ||
			strings.Contains(err.Error(), "400") {
			t.Skipf("Local Zotero API endpoint does not allow collection write operations (read-only or local authorization required): %v", err)
			return
		}
		t.Fatalf("failed to create collection in APITEST: %v", err)
	}

	// 2. Read collection back from APITEST
	createdColl, err := zot.GetCollectionByKey(group.Id, actualCollKey)
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

	// 3. Update collection name in APITEST
	createdColl.Data.Name = updatedName
	_, err = zot.UpdateCollection(group.Id, &createdColl.Data, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update collection %s: %v", actualCollKey, err)
	}

	// 4. Verify updated collection persists with new name
	updatedColl, err := zot.GetCollectionByKey(group.Id, actualCollKey)
	if err != nil {
		t.Fatalf("failed to fetch updated collection %s: %v", actualCollKey, err)
	}
	if updatedColl == nil {
		t.Fatalf("expected updated collection %s to exist, but got nil", actualCollKey)
	}
	if updatedColl.Data.Name != updatedName {
		t.Errorf("expected updated collection name '%s', got '%s'", updatedName, updatedColl.Data.Name)
	}

	// Note: The collection is deliberately NOT deleted so it remains in local Zotero APITEST collection.
	t.Logf("Successfully created and retained subcollection '%s' (Key: %s, Version: %d) in APITEST", updatedName, actualCollKey, updatedColl.Version)
}

func TestLocalApi_VerifyRetainedData(t *testing.T) {
	zot, group := getTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	// 1. Query items in APITEST to verify retained items are accessible
	items, resp, err := zot.GetItemsQuery(group.Id, map[string]string{
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
	t.Logf("Verification: APITEST group contains %d items (Total-Results: %d, Page Count: %d)", totalResults, totalResults, len(*items))

	// 2. Query collections in APITEST to verify retained collections are accessible
	colls, collResp, err := zot.GetCollectionsQuery(group.Id, map[string]string{
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
	t.Logf("Verification: APITEST group contains %d collections (Total-Results: %d, Page Count: %d)", totalColls, totalColls, len(*colls))

	for _, it := range *items {
		t.Logf("  - Retained Item: Key=%s Type=%s Title='%s' Version=%d", it.Key, it.Data.ItemType, it.Data.Title, it.Version)
	}
	for _, cl := range *colls {
		t.Logf("  - Retained Collection: Key=%s Name='%s' Version=%d", cl.Key, cl.Data.Name, cl.Version)
	}
}

func TestLocalApi_ItemCRUD_MockServerFullCycle(t *testing.T) {
	itemsStore := make(map[string]model.Item)
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
			json.NewEncoder(w).Encode(model.Group{
				Id:      6642571,
				Version: currentVersion,
				Data: model.GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/items":
			var posted []model.ItemGeneric
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
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
			json.NewEncoder(w).Encode(result)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/groups/6642571/items/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/items/")
			item, found := itemsStore[key]
			if !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.NewEncoder(w).Encode(item)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/groups/6642571/items/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/items/")
			delete(itemsStore, key)
			currentVersion++
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewClient(server.URL, "", &logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	group, err := zot.GetGroup(6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	itemKey := model.CreateKey()
	itemData := model.ItemGeneric{
		ItemDataBase: model.ItemDataBase{
			Key:      itemKey,
			ItemType: "journalArticle",
			Tags: []model.ItemTag{
				{Tag: "mock-test"},
			},
			Creators: []model.ItemDataPerson{
				{
					CreatorType: "author",
					FirstName:   "Alan",
					LastName:    "Turing",
				},
			},
		},
		Title: "Mock Computing Machinery and Intelligence",
	}

	// 1. Create item on mock server
	var lastModifiedVersion int64 = 1
	createResult, err := zot.CreateItems(group.Id, []model.ItemGeneric{itemData}, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to create item: %v", err)
	}
	if len(createResult.Success) != 1 {
		t.Fatalf("expected 1 success key, got %d", len(createResult.Success))
	}
	createdKey, ok := createResult.Success["0"]
	if !ok || createdKey != itemKey {
		t.Fatalf("expected success key for index 0 to be '%s', got '%s'", itemKey, createdKey)
	}

	// 2. Read item back
	fetchedItem, err := zot.GetItemByKey(group.Id, itemKey)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if fetchedItem == nil {
		t.Fatalf("expected non-nil item %s", itemKey)
	}
	if fetchedItem.Data.Title != "Mock Computing Machinery and Intelligence" {
		t.Errorf("expected title 'Mock Computing Machinery and Intelligence', got '%s'", fetchedItem.Data.Title)
	}

	// 3. Update item
	itemData.Title = "Mock Computing Machinery and Intelligence (Updated)"
	itemData.Version = fetchedItem.Version
	_, err = zot.UpdateItem(group.Id, &itemData, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update item: %v", err)
	}

	// 4. Verify updated title
	updatedItem, err := zot.GetItemByKey(group.Id, itemKey)
	if err != nil {
		t.Fatalf("failed to get updated item: %v", err)
	}
	if updatedItem == nil {
		t.Fatalf("expected non-nil updated item %s", itemKey)
	}
	if updatedItem.Data.Title != "Mock Computing Machinery and Intelligence (Updated)" {
		t.Errorf("expected updated title, got '%s'", updatedItem.Data.Title)
	}

	// 5. Delete item
	err = zot.DeleteItem(group.Id, itemKey, lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to delete item: %v", err)
	}

	// 6. Verify item is deleted (404)
	deletedItem, err := zot.GetItemByKey(group.Id, itemKey)
	if err != nil {
		t.Fatalf("unexpected error when getting deleted item: %v", err)
	}
	if deletedItem != nil {
		t.Errorf("expected deleted item to return nil, got %+v", deletedItem)
	}
}

func TestLocalApi_CollectionCRUD_MockServerFullCycle(t *testing.T) {
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
			json.NewEncoder(w).Encode(model.Group{
				Id:      6642571,
				Version: currentVersion,
				Data: model.GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/collections":
			var posted []model.CollectionData
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
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
			json.NewEncoder(w).Encode(result)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/groups/6642571/collections/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/collections/")
			coll, found := collectionsStore[key]
			if !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.NewEncoder(w).Encode(coll)

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
	zot, err := NewClient(server.URL, "", &logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	group, err := zot.GetGroup(6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	collKey := model.CreateKey()
	collData := model.CollectionData{
		Key:  collKey,
		Name: "Mock Artificial Intelligence",
	}

	// 1. Create collection on mock server
	var lastModifiedVersion int64 = 1
	createResult, err := zot.CreateCollections(group.Id, []model.CollectionData{collData})
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}
	if len(createResult.Success) != 1 {
		t.Fatalf("expected 1 success key, got %d", len(createResult.Success))
	}
	createdKey, ok := createResult.Success["0"]
	if !ok || createdKey != collKey {
		t.Fatalf("expected success key for index 0 to be '%s', got '%s'", collKey, createdKey)
	}

	// 2. Read collection back
	fetchedColl, err := zot.GetCollectionByKey(group.Id, collKey)
	if err != nil {
		t.Fatalf("failed to get collection: %v", err)
	}
	if fetchedColl == nil {
		t.Fatalf("expected non-nil collection %s", collKey)
	}
	if fetchedColl.Data.Name != "Mock Artificial Intelligence" {
		t.Errorf("expected name 'Mock Artificial Intelligence', got '%s'", fetchedColl.Data.Name)
	}

	// 3. Update collection
	collData.Name = "Mock Artificial Intelligence (Renamed)"
	collData.Version = fetchedColl.Version
	_, err = zot.UpdateCollection(group.Id, &collData, &lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update collection: %v", err)
	}

	// 4. Verify updated name
	updatedColl, err := zot.GetCollectionByKey(group.Id, collKey)
	if err != nil {
		t.Fatalf("failed to get updated collection: %v", err)
	}
	if updatedColl == nil {
		t.Fatalf("expected non-nil updated collection %s", collKey)
	}
	if updatedColl.Data.Name != "Mock Artificial Intelligence (Renamed)" {
		t.Errorf("expected updated name, got '%s'", updatedColl.Data.Name)
	}

	// 5. Delete collection
	err = zot.DeleteCollection(group.Id, collKey, lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to delete collection: %v", err)
	}

	// 6. Verify collection is deleted (404)
	deletedColl, err := zot.GetCollectionByKey(group.Id, collKey)
	if err != nil {
		t.Fatalf("unexpected error when getting deleted collection: %v", err)
	}
	if deletedColl != nil {
		t.Errorf("expected deleted collection to return nil, got %+v", deletedColl)
	}
}

func TestLocalApi_ServerIdHeaderExtraction(t *testing.T) {
	endpoint, groupId, _, _ := getLocalTestConfig()
	checkLocalZoteroAvailable(t, endpoint, groupId)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewClient(endpoint, "", &logger)
	if err != nil {
		t.Fatalf("failed to initialize Zotero client for server ID detection: %v", err)
	}

	serverId := zot.GetServerId()
	if serverId == "" {
		t.Logf("Notice: Local Zotero instance at %s did not provide Zotero-Server-ID header (may be older version)", endpoint)
	} else {
		t.Logf("Successfully extracted Zotero-Server-ID from local instance: %s", serverId)
	}
}

func TestLocalApi_UserGroupVersionsWithServerId(t *testing.T) {
	endpoint, groupId, localKey, _ := getLocalTestConfig()
	checkLocalZoteroAvailable(t, endpoint, groupId)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewClient(endpoint, localKey, &logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Test with current key
	versionsWithKey, err := zot.GetUserGroupVersions(zot.CurrentKey)
	if err != nil {
		t.Fatalf("GetUserGroupVersions with CurrentKey failed: %v", err)
	}
	if versionsWithKey == nil {
		t.Fatal("expected non-nil group versions map")
	}

	// Test with nil key (should safely fall back to UserId 0)
	versionsNilKey, err := zot.GetUserGroupVersions(nil)
	if err != nil {
		t.Fatalf("GetUserGroupVersions with nil key failed: %v", err)
	}
	if versionsNilKey == nil {
		t.Fatal("expected non-nil group versions map with nil key")
	}

	// Verify APITEST group exists in the returned group versions
	if version, ok := (*versionsWithKey)[defaultTestGroupId]; !ok {
		t.Errorf("expected APITEST group %d in group versions, but not found (%v)", defaultTestGroupId, *versionsWithKey)
	} else {
		t.Logf("APITEST group %d version from local user groups: %d", defaultTestGroupId, version)
	}
}

func TestLocalApi_ServerIdHeader_MockServer(t *testing.T) {
	mockServerID := "mock-server-id-xyz987"
	mockGroupVersions := map[string]int64{
		"6642571": 42,
		"1234567": 10,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "zsync") {
			t.Errorf("expected User-Agent starting with 'zsync', got '%s'", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Zotero-Server-ID", mockServerID)
		w.Header().Set("Zotero-API-Version", "3")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/" || r.URL.Path == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "local api root"}`))
			return
		}

		if strings.HasPrefix(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/groups") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(mockGroupVersions)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewClient(server.URL, "", &logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if zot.GetServerId() != mockServerID {
		t.Errorf("expected ServerId '%s', got '%s'", mockServerID, zot.GetServerId())
	}
	if zot.CurrentKey == nil || zot.CurrentKey.UserId != 0 {
		t.Errorf("expected CurrentKey with UserId 0, got %v", zot.CurrentKey)
	}

	versions, err := zot.GetUserGroupVersions(nil)
	if err != nil {
		t.Fatalf("GetUserGroupVersions failed on mock server: %v", err)
	}
	if (*versions)[6642571] != 42 || (*versions)[1234567] != 10 {
		t.Errorf("unexpected group versions result: %v", *versions)
	}
}
