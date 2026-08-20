package zotero

import (
	"encoding/json"
	"fmt"
	"github.com/je4/zsync/v2/info"
	"github.com/rs/zerolog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	defaultLocalEndpoint = "http://localhost:23119/api"
	defaultTestGroupId   = int64(6642571)
	defaultTestGroupName = "APITEST"
)

var (
	localAuthMutex sync.Mutex
	cachedLocalKey string
	localAuthDone  bool
)

func getLocalTestConfig() (string, int64, string) {
	endpoint := os.Getenv("ZOTERO_LOCAL_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultLocalEndpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	groupId := defaultTestGroupId
	if grpEnv := os.Getenv("ZOTERO_TEST_GROUP"); grpEnv != "" {
		if gid, err := strconv.ParseInt(grpEnv, 10, 64); err == nil {
			groupId = gid
		}
	}

	apiKey := os.Getenv("ZOTERO_API_KEY")
	if apiKey == "" {
		apiKey = "XxuGdxZuXiB1epXH8B9XX2oR"
	}
	return endpoint, groupId, apiKey
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

func getTestClient(t *testing.T) (*Zotero, *Group) {
	t.Helper()

	endpoint, groupId, apiKey := getLocalTestConfig()
	checkLocalZoteroAvailable(t, endpoint, groupId)

	localAuthMutex.Lock()
	if cachedLocalKey != "" && (apiKey == "XxuGdxZuXiB1epXH8B9XX2oR" || apiKey == "") {
		apiKey = cachedLocalKey
	}
	localAuthMutex.Unlock()

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewZotero(endpoint, apiKey, nil, nil, "", false, &logger, false)
	if err != nil {
		t.Fatalf("failed to create authenticated Zotero client: %v", err)
	}

	if strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1") {
		localAuthMutex.Lock()
		if !localAuthDone {
			localAuthDone = true
			if key, authErr := zot.AuthorizeLocal("ZSyncTest"); authErr == nil && key != "" {
				cachedLocalKey = key
				t.Logf("Successfully obtained local authorization key: %s", key)
			} else {
				t.Logf("Local authorization notice: %v", authErr)
			}
		} else if cachedLocalKey != "" {
			zot.SetApiKey(cachedLocalKey)
		}
		localAuthMutex.Unlock()
	}

	group, err := zot.GetGroupCloud(groupId)
	if err != nil {
		t.Fatalf("failed to retrieve test group %d: %v", groupId, err)
	}
	if group == nil {
		t.Fatalf("group %d not found on local Zotero instance", groupId)
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
	_, group := getTestClient(t)

	items, resp, err := group.GetItemsQueryCloud(map[string]string{"limit": "10"})
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
	} else if _, err := strconv.ParseInt(totalResultsStr, 10, 64); err != nil {
		t.Errorf("invalid Total-Results header '%s': %v", totalResultsStr, err)
	}

	lastModVersionStr := resp.Header().Get("Last-Modified-Version")
	if lastModVersionStr == "" {
		t.Error("expected Last-Modified-Version header in response")
	} else if _, err := strconv.ParseInt(lastModVersionStr, 10, 64); err != nil {
		t.Errorf("invalid Last-Modified-Version header '%s': %v", lastModVersionStr, err)
	}
}

func TestLocalApi_ReadAPITESTCollections(t *testing.T) {
	_, group := getTestClient(t)

	colls, resp, err := group.GetCollectionsQueryCloud(map[string]string{"limit": "10"})
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
	_, group := getTestClient(t)

	tags, lastModVer, err := group.GetTagsVersionCloud(0)
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
	_, group := getTestClient(t)

	_, resp, err := group.GetItemsQueryCloud(map[string]string{
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
	_, group := getTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	itemKey := CreateKey()
	initialTitle := "APITEST Retained Book: The Analytical Engine " + itemKey
	updatedTitle := "APITEST Retained Book: The Analytical Engine (Updated) " + itemKey

	item := &Item{
		Key:     itemKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:      itemKey,
				ItemType: "book",
				Tags: []ItemTag{
					{Tag: "apitest-retained"},
					{Tag: "local-api-test"},
				},
				Creators: []ItemDataPerson{
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
		},
	}

	// 1. Create item in APITEST (retained - no deletion in teardown)
	_, vResp, vErr := group.GetItemsQueryCloud(map[string]string{"limit": "1"})
	var lastModifiedVersion int64 = 0
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}
	err := item.UpdateCloud(&lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") {
			t.Skipf("Local Zotero API endpoint does not allow unauthenticated write operations (read-only or authorization required): %v", err)
			return
		}
		t.Fatalf("failed to create item in APITEST: %v", err)
	}

	// 2. Read item back from APITEST
	createdItem, err := group.GetItemByKeyCloud(item.Key)
	if err != nil {
		t.Fatalf("failed to fetch created item %s: %v", item.Key, err)
	}
	if createdItem == nil {
		t.Fatalf("expected created item %s to exist, but got nil", item.Key)
	}
	if createdItem.Data.Title != initialTitle {
		t.Errorf("expected title '%s', got '%s'", initialTitle, createdItem.Data.Title)
	}
	if len(createdItem.Data.Creators) != 1 || createdItem.Data.Creators[0].LastName != "Lovelace" {
		t.Errorf("creators mismatch on created item: %v", createdItem.Data.Creators)
	}
	if len(createdItem.Data.Tags) != 2 {
		t.Errorf("expected 2 tags on created item, got %d (%v)", len(createdItem.Data.Tags), createdItem.Data.Tags)
	}
	if createdItem.Version <= 0 {
		t.Errorf("expected positive version after creation, got %d", createdItem.Version)
	}

	// 3. Update item fields in APITEST
	versionBeforeUpdate := createdItem.Version
	createdItem.Data.Title = updatedTitle
	createdItem.Data.Tags = append(createdItem.Data.Tags, ItemTag{Tag: "updated-retained"})
	err = createdItem.UpdateCloud(&lastModifiedVersion)
	if err != nil {
		t.Fatalf("failed to update item %s: %v", item.Key, err)
	}

	// 4. Verify updated item persists with updated values
	updatedItem, err := group.GetItemByKeyCloud(item.Key)
	if err != nil {
		t.Fatalf("failed to fetch updated item %s: %v", item.Key, err)
	}
	if updatedItem == nil {
		t.Fatalf("expected updated item %s to exist, but got nil", item.Key)
	}
	if updatedItem.Data.Title != updatedTitle {
		t.Errorf("expected updated title '%s', got '%s'", updatedTitle, updatedItem.Data.Title)
	}
	if len(updatedItem.Data.Tags) != 3 {
		t.Errorf("expected 3 tags after update, got %d (%v)", len(updatedItem.Data.Tags), updatedItem.Data.Tags)
	}
	if updatedItem.Version <= versionBeforeUpdate {
		t.Errorf("expected item version to increment after update (old: %d, new: %d)", versionBeforeUpdate, updatedItem.Version)
	}

	// Note: The item is deliberately NOT deleted so it remains in local Zotero APITEST collection.
	t.Logf("Successfully created and retained item '%s' (Key: %s, Version: %d) in APITEST", updatedTitle, item.Key, updatedItem.Version)
}

func TestLocalApi_CreateAndRetainCollections(t *testing.T) {
	_, group := getTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	collKey := CreateKey()
	initialName := "APITEST Retained Subcollection " + collKey
	updatedName := "APITEST Retained Subcollection (Renamed) " + collKey

	coll := &Collection{
		Key:     collKey,
		Version: 0,
		Group:   group,
		Data: CollectionData{
			Key:  collKey,
			Name: initialName,
		},
	}

	// 1. Create collection in APITEST (retained - no deletion in teardown)
	err := coll.UpdateCloud()
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") {
			t.Skipf("Local Zotero API endpoint does not allow unauthenticated collection write operations (read-only or authorization required): %v", err)
			return
		}
		t.Fatalf("failed to create collection in APITEST: %v", err)
	}

	// 2. Read collection back from APITEST
	createdColl, err := group.GetCollectionByKeyCloud(coll.Key)
	if err != nil {
		t.Fatalf("failed to fetch created collection %s: %v", coll.Key, err)
	}
	if createdColl == nil {
		t.Fatalf("expected created collection %s to exist, but got nil", coll.Key)
	}
	if createdColl.Data.Name != initialName {
		t.Errorf("expected collection name '%s', got '%s'", initialName, createdColl.Data.Name)
	}
	if createdColl.Version <= 0 {
		t.Errorf("expected positive version after collection creation, got %d", createdColl.Version)
	}

	// 3. Update collection name in APITEST
	createdColl.Data.Name = updatedName
	err = createdColl.UpdateCloud()
	if err != nil {
		t.Fatalf("failed to update collection %s: %v", coll.Key, err)
	}

	// 4. Verify updated collection persists with new name
	updatedColl, err := group.GetCollectionByKeyCloud(coll.Key)
	if err != nil {
		t.Fatalf("failed to fetch updated collection %s: %v", coll.Key, err)
	}
	if updatedColl == nil {
		t.Fatalf("expected updated collection %s to exist, but got nil", coll.Key)
	}
	if updatedColl.Data.Name != updatedName {
		t.Errorf("expected updated collection name '%s', got '%s'", updatedName, updatedColl.Data.Name)
	}

	// Note: The collection is deliberately NOT deleted so it remains in local Zotero APITEST collection.
	t.Logf("Successfully created and retained subcollection '%s' (Key: %s, Version: %d) in APITEST", updatedName, coll.Key, updatedColl.Version)
}

func TestLocalApi_VerifyRetainedData(t *testing.T) {
	_, group := getTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	// 1. Query items in APITEST to verify retained items are accessible
	items, resp, err := group.GetItemsQueryCloud(map[string]string{
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
	colls, collResp, err := group.GetCollectionsQueryCloud(map[string]string{
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
	itemsStore := make(map[string]Item)
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
			json.NewEncoder(w).Encode(Group{
				Id:      6642571,
				Version: currentVersion,
				Data: GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/items":
			var posted []ItemGeneric
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result := ItemCollectionCreateResult{
				Success:    make(map[string]string),
				Successful: make(map[string]Item),
				Unchanged:  make(map[string]string),
				Failed:     make(map[string]ItemCollectionCreateResultFailed),
			}
			currentVersion++
			for idx, itemData := range posted {
				key := itemData.Key
				if key == "" {
					key = CreateKey()
					itemData.Key = key
				}
				item := Item{
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
	zot, err := NewZotero(server.URL, "", nil, nil, "public", false, &logger, false)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	group, err := zot.GetGroupCloud(6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	itemKey := CreateKey()
	item := &Item{
		Key:     itemKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:      itemKey,
				ItemType: "book",
				Tags: []ItemTag{
					{Tag: "unit-tag"},
				},
				Creators: []ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Ada",
						LastName:    "Lovelace",
					},
				},
			},
			Title: "Test Mock Book",
		},
	}

	var lastMod int64 = 0
	if err := item.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("UpdateCloud (create) failed: %v", err)
	}

	created, err := group.GetItemByKeyCloud(item.Key)
	if err != nil || created == nil {
		t.Fatalf("GetItemByKeyCloud failed: %v, created: %v", err, created)
	}
	if created.Data.Title != "Test Mock Book" {
		t.Errorf("expected 'Test Mock Book', got '%s'", created.Data.Title)
	}

	// Update
	created.Data.Title = "Test Mock Book (Updated)"
	created.Data.Tags = append(created.Data.Tags, ItemTag{Tag: "tag-2"})
	if err := created.UpdateCloud(&created.Version); err != nil {
		t.Fatalf("UpdateCloud (update) failed: %v", err)
	}

	updated, err := group.GetItemByKeyCloud(item.Key)
	if err != nil || updated == nil {
		t.Fatalf("GetItemByKeyCloud after update failed: %v", err)
	}
	if updated.Data.Title != "Test Mock Book (Updated)" {
		t.Errorf("expected updated title, got '%s'", updated.Data.Title)
	}
	if len(updated.Data.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(updated.Data.Tags))
	}

	// Delete
	if err := updated.DeleteCloud(updated.Version); err != nil {
		t.Fatalf("DeleteCloud failed: %v", err)
	}

	deleted, err := group.GetItemByKeyCloud(item.Key)
	if err != nil {
		t.Fatalf("GetItemByKeyCloud after delete returned error: %v", err)
	}
	if deleted != nil {
		t.Errorf("expected nil after delete, got %v", deleted)
	}
}

func TestLocalApi_CollectionCRUD_MockServerFullCycle(t *testing.T) {
	collsStore := make(map[string]Collection)
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
			json.NewEncoder(w).Encode(Group{
				Id:      6642571,
				Version: currentVersion,
				Data: GroupData{
					Id:   6642571,
					Name: "APITEST",
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/collections":
			var posted []CollectionData
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result := ItemCollectionCreateResult{
				Success:    make(map[string]string),
				Successful: make(map[string]Item),
				Unchanged:  make(map[string]string),
				Failed:     make(map[string]ItemCollectionCreateResultFailed),
			}
			currentVersion++
			for idx, collData := range posted {
				key := collData.Key
				if key == "" {
					key = CreateKey()
					collData.Key = key
				}
				coll := Collection{
					Key:     key,
					Version: currentVersion,
					Data:    collData,
				}
				collsStore[key] = coll
				idxStr := strconv.Itoa(idx)
				result.Success[idxStr] = key
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.NewEncoder(w).Encode(result)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/groups/6642571/collections/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/collections/")
			coll, found := collsStore[key]
			if !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.NewEncoder(w).Encode(coll)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/groups/6642571/collections/"):
			key := strings.TrimPrefix(r.URL.Path, "/groups/6642571/collections/")
			delete(collsStore, key)
			currentVersion++
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewZotero(server.URL, "", nil, nil, "public", false, &logger, false)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	group, err := zot.GetGroupCloud(6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	collKey := CreateKey()
	coll := &Collection{
		Key:     collKey,
		Version: 0,
		Group:   group,
		Data: CollectionData{
			Key:  collKey,
			Name: "Mock Subcollection",
		},
	}

	if err := coll.UpdateCloud(); err != nil {
		t.Fatalf("UpdateCloud (create collection) failed: %v", err)
	}

	created, err := group.GetCollectionByKeyCloud(coll.Key)
	if err != nil || created == nil {
		t.Fatalf("GetCollectionByKeyCloud failed: %v", err)
	}
	if created.Data.Name != "Mock Subcollection" {
		t.Errorf("expected 'Mock Subcollection', got '%s'", created.Data.Name)
	}

	// Update collection name
	created.Data.Name = "Mock Subcollection (Renamed)"
	if err := created.UpdateCloud(); err != nil {
		t.Fatalf("UpdateCloud (update collection) failed: %v", err)
	}

	updated, err := group.GetCollectionByKeyCloud(coll.Key)
	if err != nil || updated == nil {
		t.Fatalf("GetCollectionByKeyCloud after update failed: %v", err)
	}
	if updated.Data.Name != "Mock Subcollection (Renamed)" {
		t.Errorf("expected updated name, got '%s'", updated.Data.Name)
	}

	// Delete collection
	if err := updated.DeleteCloud(updated.Version); err != nil {
		t.Fatalf("DeleteCloud collection failed: %v", err)
	}

	deleted, err := group.GetCollectionByKeyCloud(coll.Key)
	if err != nil {
		t.Fatalf("GetCollectionByKeyCloud after delete returned error: %v", err)
	}
	if deleted != nil {
		t.Errorf("expected nil after delete, got %v", deleted)
	}
}

func TestLocalApi_ServerIdHeaderExtraction(t *testing.T) {
	zot, _ := getTestClient(t)

	serverId := zot.GetServerId()
	if serverId == "" {
		t.Errorf("expected non-empty ServerId extracted from local Zotero response headers, got empty string")
	} else {
		t.Logf("Detected Zotero Server ID: %s", serverId)
	}

	// Explicitly invoke DetectServerId
	detectedId, err := zot.DetectServerId()
	if err != nil {
		t.Fatalf("DetectServerId returned error: %v", err)
	}
	if detectedId != serverId {
		t.Errorf("expected DetectServerId to return '%s', got '%s'", serverId, detectedId)
	}

	// Verify unauthenticated CurrentKey is initialized
	if zot.CurrentKey == nil {
		t.Fatal("expected zot.CurrentKey to be initialized in unauthenticated mode")
	}
	if zot.CurrentKey.UserId != 0 {
		t.Errorf("expected default UserId 0 for local unauthenticated operation, got %d", zot.CurrentKey.UserId)
	}
}

func TestLocalApi_UserGroupVersionsWithServerId(t *testing.T) {
	zot, _ := getTestClient(t)

	// Test with explicit CurrentKey
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
	zot, err := NewZotero(server.URL, "", nil, nil, "", false, &logger, false)
	if err != nil {
		t.Fatalf("NewZotero failed: %v", err)
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
