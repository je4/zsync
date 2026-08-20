package zotero

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/je4/zsync/v2/info"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

const (
	defaultCloudEndpoint = "https://api.zotero.org"
)

func getCloudTestConfig() (string, int64, string) {
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
	if apiKey == "" {
		apiKey = "OVWI72PWcdt4t5NcJKGARHb6"
	}
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
			if bSec, err := strconv.ParseInt(backoffStr, 10, 64); err == nil && bSec > 0 {
				if bSec > 15 {
					bSec = 15
				}
				t.Logf("Cloud Zotero API requested backoff: %d seconds", bSec)
				time.Sleep(time.Duration(bSec) * time.Second)
			}
		}

		// Check rate limiting / service maintenance (429 / 503)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			retryAfter := int64(0)
			if retryAfterStr := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfterStr != "" {
				if val, err := strconv.ParseInt(retryAfterStr, 10, 64); err == nil {
					retryAfter = val
				} else if targetTime, err := http.ParseTime(retryAfterStr); err == nil {
					diff := time.Until(targetTime)
					if diff > 0 {
						retryAfter = int64(diff / time.Second)
						if diff%time.Second > 0 {
							retryAfter++
						}
					}
				}
			}

			resp.Body.Close()

			if retryAfter > 0 && attempt < maxAttempts {
				if retryAfter > 15 {
					retryAfter = 15
				}
				t.Logf("Cloud Zotero API rate limited (HTTP %d), waiting %d seconds before retry (attempt %d/%d)...", resp.StatusCode, retryAfter, attempt, maxAttempts)
				time.Sleep(time.Duration(retryAfter) * time.Second)
				continue
			}

			t.Logf("Cloud Zotero API returned status %d at %s. Skipping integration test.", resp.StatusCode, probeUrl)
			t.Skipf("Cloud Zotero API returned status %d at %s (rate-limited/unavailable) - skipping test", resp.StatusCode, probeUrl)
			return
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("Cloud Zotero API returned status %d at %s. Skipping integration test.", resp.StatusCode, probeUrl)
			t.Skipf("Cloud Zotero API returned status %d at %s (unauthorized, rate-limited, or unavailable) - skipping test", resp.StatusCode, probeUrl)
			return
		}

		return
	}
}

func getCloudTestClient(t *testing.T) (*Zotero, *Group) {
	t.Helper()

	endpoint, groupId, apiKey := getCloudTestConfig()
	checkCloudZoteroAvailable(t, endpoint, groupId, apiKey)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewZotero(endpoint, apiKey, nil, nil, "", false, &logger, false)
	if err != nil {
		t.Fatalf("failed to create authenticated Cloud Zotero client: %v", err)
	}

	group, err := zot.GetGroupCloud(groupId)
	if err != nil {
		t.Fatalf("failed to retrieve cloud test group %d: %v", groupId, err)
	}
	if group == nil {
		t.Fatalf("group %d not found on cloud Zotero instance", groupId)
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

func TestCloudApi_ReadAPITESTItems(t *testing.T) {
	_, group := getCloudTestClient(t)

	items, resp, err := group.GetItemsQueryCloud(map[string]string{"limit": "10"})
	if err != nil {
		t.Fatalf("failed to query items for cloud group %d: %v", group.Id, err)
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

func TestCloudApi_ReadAPITESTCollections(t *testing.T) {
	_, group := getCloudTestClient(t)

	colls, resp, err := group.GetCollectionsQueryCloud(map[string]string{"limit": "10"})
	if err != nil {
		t.Fatalf("failed to query collections for cloud group %d: %v", group.Id, err)
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
	_, group := getCloudTestClient(t)

	tags, lastModVer, err := group.GetTagsVersionCloud(0)
	if err != nil {
		t.Fatalf("failed to query tags for cloud group %d: %v", group.Id, err)
	}
	if tags == nil {
		t.Fatal("expected non-nil tags slice")
	}
	if lastModVer <= 0 {
		t.Logf("last modified version for tags: %d", lastModVer)
	}
}

func TestCloudApi_PaginationAndFilters(t *testing.T) {
	_, group := getCloudTestClient(t)

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

func TestCloudApi_CreateAndRetainItems(t *testing.T) {
	_, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	itemKey := CreateKey()
	initialTitle := "APITEST Cloud Retained Book: The Analytical Engine " + itemKey
	updatedTitle := "APITEST Cloud Retained Book: The Analytical Engine (Updated) " + itemKey

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
					{Tag: "cloud-api-test"},
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
			AbstractNote: "Sample retained book entry created by automated tests for Zotero Cloud inspection.",
		},
	}

	// 1. Create item in Cloud APITEST (retained - no deletion in teardown)
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
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") {
			t.Skipf("Cloud Zotero API endpoint does not allow write operations (unauthorized/forbidden): %v", err)
			return
		}
		t.Fatalf("failed to create item in Cloud APITEST: %v", err)
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

	// 3. Update item fields in Cloud APITEST
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

	t.Logf("Successfully created and retained item '%s' (Key: %s, Version: %d) in Cloud APITEST", updatedTitle, item.Key, updatedItem.Version)
}

func TestCloudApi_CreateAndRetainCollections(t *testing.T) {
	_, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	collKey := CreateKey()
	initialName := "APITEST Cloud Retained Subcollection " + collKey
	updatedName := "APITEST Cloud Retained Subcollection (Renamed) " + collKey

	coll := &Collection{
		Key:     collKey,
		Version: 0,
		Group:   group,
		Data: CollectionData{
			Key:  collKey,
			Name: initialName,
		},
	}

	// 1. Create collection in Cloud APITEST (retained - no deletion in teardown)
	err := coll.UpdateCloud()
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") {
			t.Skipf("Cloud Zotero API endpoint does not allow collection write operations (unauthorized/forbidden): %v", err)
			return
		}
		t.Fatalf("failed to create collection in Cloud APITEST: %v", err)
	}

	// 2. Read collection back from Cloud APITEST
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

	// 3. Update collection name in Cloud APITEST
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

	t.Logf("Successfully created and retained subcollection '%s' (Key: %s, Version: %d) in Cloud APITEST", updatedName, coll.Key, updatedColl.Version)
}

func TestCloudApi_CreateAndRetainAttachment(t *testing.T) {
	zot, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	// Initialize local filesystem store if not configured
	if zot.Fs == nil {
		tempDir := t.TempDir()
		fsLogger := logging.MustGetLogger("test")
		localFs, err := filesystem.NewLocalFs(tempDir, fsLogger)
		if err != nil {
			t.Fatalf("failed to create local fs: %v", err)
		}
		zot.Fs = localFs
	}

	bucket, err := group.GetFolder()
	if err != nil {
		t.Fatalf("failed to get group bucket: %v", err)
	}

	// 1. Create parent book item in Cloud APITEST
	parentKey := CreateKey()
	parentTitle := "APITEST Cloud Book with Attachment " + parentKey
	parentItem := &Item{
		Key:     parentKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:      parentKey,
				ItemType: "book",
				Tags: []ItemTag{
					{Tag: "apitest-retained"},
					{Tag: "attachment-parent"},
				},
				Creators: []ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Charles",
						LastName:    "Babbage",
					},
				},
			},
			Title:        parentTitle,
			AbstractNote: "Parent book entry created to attach binary document in APITEST.",
		},
	}

	var lastModifiedVersion int64 = 0
	_, vResp, vErr := group.GetItemsQueryCloud(map[string]string{"limit": "1"})
	if vErr == nil && vResp != nil {
		if lmv, err := strconv.ParseInt(vResp.Header().Get("Last-Modified-Version"), 10, 64); err == nil {
			lastModifiedVersion = lmv
		}
	}
	if lastModifiedVersion <= 0 {
		lastModifiedVersion = group.Version
	}

	err = parentItem.UpdateCloud(&lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") {
			t.Skipf("Cloud Zotero API endpoint does not allow write operations: %v", err)
			return
		}
		t.Fatalf("failed to create parent item in Cloud APITEST: %v", err)
	}

	// 2. Prepare binary attachment file in zot.Fs
	attKey := CreateKey()
	fileContent := []byte(fmt.Sprintf("%%PDF-1.4 APITEST binary attachment created at %s for key %s\n", time.Now().Format(time.RFC3339), attKey))
	expectedMD5 := fmt.Sprintf("%x", md5.Sum(fileContent))
	filename := fmt.Sprintf("notes-%s.pdf", attKey)

	if err := zot.Fs.FilePut(bucket, attKey, fileContent, filesystem.FilePutOptions{ContentType: "application/pdf"}); err != nil {
		t.Fatalf("failed to write local attachment file: %v", err)
	}

	// 3. Create child attachment item and upload to Cloud APITEST
	attTitle := "Analytical Engine Diagram " + attKey
	attItem := &Item{
		Key:     attKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:        attKey,
				ItemType:   "attachment",
				ParentItem: parentItem.Key,
				Tags: []ItemTag{
					{Tag: "apitest-retained"},
					{Tag: "cloud-attachment"},
				},
			},
			Title:       attTitle,
			LinkMode:    "imported_file",
			ContentType: "application/pdf",
			Filename:    filename,
		},
	}

	err = attItem.UpdateCloud(&lastModifiedVersion)
	if err != nil {
		if strings.Contains(err.Error(), "Endpoint does not support method") ||
			strings.Contains(err.Error(), "does not support method") ||
			strings.Contains(err.Error(), "API key required") ||
			strings.Contains(err.Error(), "401") ||
			strings.Contains(err.Error(), "403") ||
			strings.Contains(err.Error(), "Cannot edit item") ||
			strings.Contains(err.Error(), "storage") {
			t.Logf("Attachment creation/upload in Cloud APITEST restricted by API key permissions or storage quota: %v", err)
			t.Skipf("Cloud Zotero API endpoint does not allow attachment creation/upload: %v", err)
			return
		}
		t.Fatalf("failed to create and upload attachment item in Cloud APITEST: %v", err)
	}

	// 4. Fetch attachment item from Cloud APITEST to verify parent-child relationship
	createdAtt, err := group.GetItemByKeyCloud(attItem.Key)
	if err != nil {
		t.Fatalf("failed to fetch created attachment %s: %v", attItem.Key, err)
	}
	if createdAtt == nil {
		t.Fatalf("expected created attachment %s to exist, but got nil", attItem.Key)
	}
	if createdAtt.Data.ParentItem != parentItem.Key {
		t.Errorf("expected parentItem '%s', got '%s'", parentItem.Key, createdAtt.Data.ParentItem)
	}
	if createdAtt.Data.ItemType != "attachment" {
		t.Errorf("expected itemType 'attachment', got '%s'", createdAtt.Data.ItemType)
	}

	// 5. Test download if file was uploaded to storage
	if attItem.MD5 != "" {
		dlTempDir := t.TempDir()
		dlFs, _ := filesystem.NewLocalFs(dlTempDir, logging.MustGetLogger("test"))
		endpoint, groupId, apiKey := getCloudTestConfig()
		clientLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		dlZot, err := NewZotero(endpoint, apiKey, nil, dlFs, "", false, &clientLogger, false)
		if err == nil {
			dlGroup, err := dlZot.GetGroupCloud(groupId)
			if err == nil {
				dlItem := &Item{
					Key:     attItem.Key,
					Version: attItem.Version,
					Group:   dlGroup,
					Data:    attItem.Data,
				}
				dlMD5, err := dlItem.DownloadAttachmentCloud()
				if err != nil {
					t.Logf("DownloadAttachmentCloud result: %v", err)
				} else if dlMD5 != expectedMD5 && dlMD5 != attItem.MD5 {
					t.Errorf("expected downloaded MD5 '%s', got '%s'", expectedMD5, dlMD5)
				}
			}
		}
	}

	t.Logf("Successfully created and retained attachment '%s' (Key: %s, Parent: %s, Version: %d) in Cloud APITEST", attTitle, attItem.Key, parentItem.Key, attItem.Version)
}

func TestCloudApi_VerifyRetainedData(t *testing.T) {
	_, group := getCloudTestClient(t)

	// Ensure we operate strictly within APITEST group
	if group.Id != defaultTestGroupId {
		t.Fatalf("safety guard: target group ID %d does not match APITEST group ID %d", group.Id, defaultTestGroupId)
	}

	// 1. Query items in APITEST to verify retained items are accessible
	items, resp, err := group.GetItemsQueryCloud(map[string]string{
		"limit": "25",
	})
	if err != nil {
		t.Fatalf("failed to query items for verification in Cloud APITEST: %v", err)
	}
	if items == nil {
		t.Fatal("expected non-nil items list during retention verification")
	}

	totalResultsStr := resp.Header().Get("Total-Results")
	totalResults, _ := strconv.ParseInt(totalResultsStr, 10, 64)
	t.Logf("Cloud Verification: APITEST group contains %d items (Total-Results: %d, Page Count: %d)", totalResults, totalResults, len(*items))

	// 2. Query collections in APITEST to verify retained collections are accessible
	colls, collResp, err := group.GetCollectionsQueryCloud(map[string]string{
		"limit": "25",
	})
	if err != nil {
		t.Fatalf("failed to query collections for verification in Cloud APITEST: %v", err)
	}
	if colls == nil {
		t.Fatal("expected non-nil collections list during retention verification")
	}

	totalCollsStr := collResp.Header().Get("Total-Results")
	totalColls, _ := strconv.ParseInt(totalCollsStr, 10, 64)
	t.Logf("Cloud Verification: APITEST group contains %d collections (Total-Results: %d, Page Count: %d)", totalColls, totalColls, len(*colls))

	for _, it := range *items {
		if it.Data.ItemType == "attachment" {
			t.Logf("  - Cloud Retained Attachment: Key=%s Parent=%s Filename='%s' Title='%s' Version=%d", it.Key, it.Data.ParentItem, it.Data.Filename, it.Data.Title, it.Version)
		} else {
			t.Logf("  - Cloud Retained Item: Key=%s Type=%s Title='%s' Version=%d", it.Key, it.Data.ItemType, it.Data.Title, it.Version)
		}
	}
	for _, cl := range *colls {
		t.Logf("  - Cloud Retained Collection: Key=%s Name='%s' Version=%d", cl.Key, cl.Data.Name, cl.Version)
	}
}

func TestCloudApi_MockServerFullCycle(t *testing.T) {
	itemsStore := make(map[string]Item)
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

		case r.Method == http.MethodGet && r.URL.Path == "/groups/6642571/items":
			w.Header().Set("Total-Results", strconv.Itoa(len(itemsStore)))
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			var list []ItemGeneric
			for _, it := range itemsStore {
				list = append(list, it.Data)
			}
			json.NewEncoder(w).Encode(list)

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
				itemData.Version = currentVersion
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
			it, found := itemsStore[key]
			if !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified-Version", strconv.FormatInt(currentVersion, 10))
			json.NewEncoder(w).Encode(it)

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

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewZotero(server.URL, "mock-api-key", nil, nil, "", false, &logger, false)
	if err != nil {
		t.Fatalf("failed to create cloud client: %v", err)
	}

	group, err := zot.GetGroupCloud(6642571)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}
	if group == nil {
		t.Fatal("expected non-nil group")
	}

	// 1. Create item via cloud API client
	itemKey := CreateKey()
	item := &Item{
		Key:     itemKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:      itemKey,
				ItemType: "book",
				Tags:     []ItemTag{{Tag: "cloud-mock-tag"}},
				Creators: []ItemDataPerson{
					{
						CreatorType: "author",
						FirstName:   "Ada",
						LastName:    "Lovelace",
					},
				},
			},
			Title: "Mock Cloud Book",
		},
	}

	var lastMod int64 = 0
	if err := item.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("UpdateCloud item failed: %v", err)
	}

	createdItem, err := group.GetItemByKeyCloud(item.Key)
	if err != nil || createdItem == nil {
		t.Fatalf("GetItemByKeyCloud failed: %v", err)
	}
	if createdItem.Data.Title != "Mock Cloud Book" {
		t.Errorf("expected title 'Mock Cloud Book', got '%s'", createdItem.Data.Title)
	}

	// 2. Create collection via cloud API client
	collKey := CreateKey()
	coll := &Collection{
		Key:     collKey,
		Version: 0,
		Group:   group,
		Data: CollectionData{
			Key:  collKey,
			Name: "Mock Cloud Collection",
		},
	}

	if err := coll.UpdateCloud(); err != nil {
		t.Fatalf("UpdateCloud collection failed: %v", err)
	}

	createdColl, err := group.GetCollectionByKeyCloud(coll.Key)
	if err != nil || createdColl == nil {
		t.Fatalf("GetCollectionByKeyCloud failed: %v", err)
	}
	if createdColl.Data.Name != "Mock Cloud Collection" {
		t.Errorf("expected collection name 'Mock Cloud Collection', got '%s'", createdColl.Data.Name)
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
			json.NewEncoder(w).Encode(Group{
				Id:      6642571,
				Version: currentVersion,
				Data: GroupData{
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
			result := ItemCollectionCreateResult{
				Success:    map[string]string{"0": "ITEM123"},
				Successful: map[string]Item{"0": {Key: "ITEM123", Version: currentVersion}},
				Unchanged:  map[string]string{},
				Failed:     map[string]ItemCollectionCreateResultFailed{},
			}
			json.NewEncoder(w).Encode(result)

		case r.Method == http.MethodDelete && r.URL.Path == "/groups/6642571/items/ITEM123":
			deleteItemCalls++
			if deleteItemCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
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
			result := ItemCollectionCreateResult{
				Success:    map[string]string{"0": "COLL123"},
				Successful: map[string]Item{},
				Unchanged:  map[string]string{},
				Failed:     map[string]ItemCollectionCreateResultFailed{},
			}
			json.NewEncoder(w).Encode(result)

		case r.Method == http.MethodDelete && r.URL.Path == "/groups/6642571/collections/COLL123":
			deleteCollCalls++
			if deleteCollCalls == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 1. Verify checkCloudZoteroAvailable retries probe on 429 + Retry-After and honors Backoff
	checkCloudZoteroAvailable(t, server.URL, 6642571, "test-api-key")
	if probeCalls < 2 {
		t.Errorf("expected at least 2 probe calls with retry, got %d", probeCalls)
	}

	logger := zerolog.Nop()
	zot, err := NewZotero(server.URL, "test-api-key", nil, nil, "", false, &logger, false)
	if err != nil {
		t.Fatalf("failed to create zotero client: %v", err)
	}

	// 2. Verify GetGroupCloud retries on 429
	group, err := zot.GetGroupCloud(6642571)
	if err != nil {
		t.Fatalf("GetGroupCloud failed: %v", err)
	}
	if getGroupCalls < 2 {
		t.Errorf("expected at least 2 GetGroupCloud calls with retry, got %d", getGroupCalls)
	}

	// 3. Verify Item.UpdateCloud and DeleteCloud retry on 429
	item := &Item{
		Key:     "ITEM123",
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			Title: "Test Retry Item",
		},
	}
	var lastMod int64 = 1
	if err := item.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("Item.UpdateCloud failed: %v", err)
	}
	if postItemCalls < 2 {
		t.Errorf("expected at least 2 postItemCalls with retry, got %d", postItemCalls)
	}

	if err := item.DeleteCloud(lastMod); err != nil {
		t.Fatalf("Item.DeleteCloud failed: %v", err)
	}
	if deleteItemCalls < 2 {
		t.Errorf("expected at least 2 deleteItemCalls with retry, got %d", deleteItemCalls)
	}

	// 4. Verify Collection.UpdateCloud and DeleteCloud retry on 429
	coll := &Collection{
		Key:     "COLL123",
		Version: 0,
		Group:   group,
		Data: CollectionData{
			Name: "Test Retry Collection",
		},
	}
	if err := coll.UpdateCloud(); err != nil {
		t.Fatalf("Collection.UpdateCloud failed: %v", err)
	}
	if postCollCalls < 2 {
		t.Errorf("expected at least 2 postCollCalls with retry, got %d", postCollCalls)
	}

	if err := coll.DeleteCloud(lastMod); err != nil {
		t.Fatalf("Collection.DeleteCloud failed: %v", err)
	}
	if deleteCollCalls < 2 {
		t.Errorf("expected at least 2 deleteCollCalls with retry, got %d", deleteCollCalls)
	}
}

func TestCloudApi_AttachmentUploadAndDownload_MockServer(t *testing.T) {
	uploadedStorageFiles := make(map[string][]byte)
	itemsStore := make(map[string]Item)
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
			json.NewEncoder(w).Encode(Group{
				Id:      6642571,
				Version: currentVersion,
				Data: GroupData{
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
			itemsList := make([]Item, 0, len(itemsStore))
			for _, it := range itemsStore {
				itemsList = append(itemsList, it)
			}
			json.NewEncoder(w).Encode(itemsList)
			return
		}

		// 3. Create items: POST /groups/6642571/items
		if r.Method == http.MethodPost && r.URL.Path == "/groups/6642571/items" {
			w.Header().Set("Content-Type", "application/json")
			var rawItems []ItemGeneric
			if err := json.NewDecoder(r.Body).Decode(&rawItems); err != nil {
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
				item := Item{
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
			json.NewEncoder(w).Encode(respMap)
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
					json.NewEncoder(w).Encode(map[string]string{
						"exists": "1",
					})
					return
				}

				// Return upload authorization pointing to mock S3 / storage endpoint
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{
					"url":         serverURL + "/mock-storage/" + itemKey,
					"contentType": "application/octet-stream",
					"prefix":      "PREFIX_DATA_",
					"suffix":      "_SUFFIX_DATA",
					"uploadKey":   "UPKEY_" + itemKey,
				})
				return
			}
		}

		// 5. Mock Storage Endpoint (Step 2): POST /mock-storage/{key}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/mock-storage/") {
			parts := strings.Split(r.URL.Path, "/")
			itemKey := parts[2]
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read storage upload body: %v", err)
			}
			prefix := "PREFIX_DATA_"
			suffix := "_SUFFIX_DATA"
			if !bytes.HasPrefix(bodyBytes, []byte(prefix)) || !bytes.HasSuffix(bodyBytes, []byte(suffix)) {
				t.Errorf("storage payload missing expected prefix/suffix: %s", string(bodyBytes))
			}
			extractedData := bodyBytes[len(prefix) : len(bodyBytes)-len(suffix)]
			uploadedStorageFiles[itemKey] = extractedData
			w.WriteHeader(http.StatusCreated)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	serverURL = server.URL

	// Set up temporary local filesystem
	tempDir := t.TempDir()
	fsLogger := logging.MustGetLogger("test")
	localFs, err := filesystem.NewLocalFs(tempDir, fsLogger)
	if err != nil {
		t.Fatalf("failed to create temporary LocalFs: %v", err)
	}

	clientLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	zot, err := NewZotero(server.URL, "TEST_API_KEY", nil, localFs, "", false, &clientLogger, false)
	if err != nil {
		t.Fatalf("failed to create Zotero client: %v", err)
	}

	group, err := zot.GetGroupCloud(6642571)
	if err != nil {
		t.Fatalf("failed to get group from mock server: %v", err)
	}

	bucket, err := group.GetFolder()
	if err != nil {
		t.Fatalf("failed to get group bucket: %v", err)
	}

	// 1. Create parent book item
	parentKey := "BOOK_PARENT_01"
	parentItem := &Item{
		Key:     parentKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:      parentKey,
				ItemType: "book",
			},
			Title: "Parent Research Book",
		},
	}
	var lastMod int64 = 1
	if err := parentItem.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("failed to create parent book item: %v", err)
	}

	// 2. Prepare attachment binary data and save in localFs
	attKey := "ATT_NEW_001"
	rawBinaryPayload := []byte("%PDF-1.4 sample binary file content for zsync mock test attachment")
	expectedMD5 := fmt.Sprintf("%x", md5.Sum(rawBinaryPayload))

	if err := localFs.FilePut(bucket, attKey, rawBinaryPayload, filesystem.FilePutOptions{ContentType: "application/pdf"}); err != nil {
		t.Fatalf("failed to write local attachment file before upload: %v", err)
	}

	// 3. Create child attachment item and upload
	attItem := &Item{
		Key:     attKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:        attKey,
				ItemType:   "attachment",
				ParentItem: parentItem.Key,
			},
			Title:       "Research Attachment Document.pdf",
			LinkMode:    "imported_file",
			ContentType: "application/pdf",
			Filename:    "document.pdf",
		},
	}

	if err := attItem.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("failed to create and upload attachment item: %v", err)
	}

	// Verify attItem MD5 was updated
	if attItem.MD5 != expectedMD5 {
		t.Errorf("expected attachment item MD5 '%s', got '%s'", expectedMD5, attItem.MD5)
	}

	// Verify storage endpoint received and extracted exact binary content under assigned key
	storedBytes, ok := uploadedStorageFiles[attItem.Key]
	if !ok {
		t.Fatalf("expected file '%s' to be stored in mock storage", attItem.Key)
	}
	if !bytes.Equal(storedBytes, rawBinaryPayload) {
		t.Errorf("stored bytes do not match original payload: got %s, expected %s", string(storedBytes), string(rawBinaryPayload))
	}

	// 4. Download attachment via DownloadAttachmentCloud
	tempDir2 := t.TempDir()
	localFs2, err := filesystem.NewLocalFs(tempDir2, fsLogger)
	if err != nil {
		t.Fatalf("failed to create second temporary LocalFs: %v", err)
	}
	zot2, err := NewZotero(server.URL, "TEST_API_KEY", nil, localFs2, "", false, &clientLogger, false)
	if err != nil {
		t.Fatalf("failed to create second Zotero client: %v", err)
	}
	group2, err := zot2.GetGroupCloud(6642571)
	if err != nil {
		t.Fatalf("failed to get group2: %v", err)
	}

	attItemForDownload := &Item{
		Key:     attItem.Key,
		Version: attItem.Version,
		Group:   group2,
		Data:    attItem.Data,
	}

	dlMD5, err := attItemForDownload.DownloadAttachmentCloud()
	if err != nil {
		t.Fatalf("DownloadAttachmentCloud failed: %v", err)
	}
	if dlMD5 != expectedMD5 {
		t.Errorf("expected downloaded MD5 '%s', got '%s'", expectedMD5, dlMD5)
	}

	bucket2, _ := group2.GetFolder()
	downloadedBytes, err := localFs2.FileGet(bucket2, attItem.Key, filesystem.FileGetOptions{})
	if err != nil {
		t.Fatalf("failed to read downloaded file from localFs2: %v", err)
	}
	if !bytes.Equal(downloadedBytes, rawBinaryPayload) {
		t.Errorf("downloaded bytes mismatch: got %s, expected %s", string(downloadedBytes), string(rawBinaryPayload))
	}

	// 5. Test exists == 1 case
	existKey := "EXISTING_ATT"
	if err := localFs.FilePut(bucket, existKey, rawBinaryPayload, filesystem.FilePutOptions{ContentType: "application/pdf"}); err != nil {
		t.Fatalf("failed to write existing test file: %v", err)
	}
	existItem := &Item{
		Key:     existKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:        existKey,
				ItemType:   "attachment",
				ParentItem: parentItem.Key,
			},
			Title:       "Existing Attachment.pdf",
			LinkMode:    "imported_file",
			ContentType: "application/pdf",
			Filename:    "existing.pdf",
		},
	}
	if err := existItem.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("expected UpdateCloud with exists:1 to succeed, got: %v", err)
	}

	// 6. Test rate limiting retry (HTTP 429) during upload auth
	retryKey := "RETRY_ATT"
	if err := localFs.FilePut(bucket, retryKey, rawBinaryPayload, filesystem.FilePutOptions{ContentType: "application/pdf"}); err != nil {
		t.Fatalf("failed to write retry test file: %v", err)
	}
	retryItem := &Item{
		Key:     retryKey,
		Version: 0,
		Group:   group,
		Data: ItemGeneric{
			ItemDataBase: ItemDataBase{
				Key:        retryKey,
				ItemType:   "attachment",
				ParentItem: parentItem.Key,
			},
			Title:       "Retry Attachment.pdf",
			LinkMode:    "imported_file",
			ContentType: "application/pdf",
			Filename:    "retry.pdf",
		},
	}
	if err := retryItem.UpdateCloud(&lastMod); err != nil {
		t.Fatalf("expected UpdateCloud with 429 retry to succeed, got: %v", err)
	}
	if authAttempts < 1 {
		t.Errorf("expected at least 1 authAttempt with 429 retry, got %d", authAttempts)
	}
}
