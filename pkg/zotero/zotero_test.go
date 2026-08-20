package zotero

import (
	"encoding/json"
	"github.com/je4/zsync/v2/info"
	"github.com/rs/zerolog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestItemDataPersonSerialization(t *testing.T) {
	// Single-field creator (organization/institution)
	singleFieldJSON := `{"creatorType":"author","name":"World Health Organization"}`
	var person1 ItemDataPerson
	if err := json.Unmarshal([]byte(singleFieldJSON), &person1); err != nil {
		t.Fatalf("failed to unmarshal single-field creator: %v", err)
	}
	if person1.Name != "World Health Organization" {
		t.Errorf("expected Name to be 'World Health Organization', got '%s'", person1.Name)
	}
	if person1.FirstName != "" || person1.LastName != "" {
		t.Errorf("expected FirstName and LastName to be empty, got '%s', '%s'", person1.FirstName, person1.LastName)
	}

	marshaled1, err := json.Marshal(person1)
	if err != nil {
		t.Fatalf("failed to marshal single-field creator: %v", err)
	}
	if string(marshaled1) != singleFieldJSON {
		t.Errorf("expected JSON '%s', got '%s'", singleFieldJSON, string(marshaled1))
	}

	// Two-field creator (person)
	twoFieldJSON := `{"creatorType":"author","firstName":"Ada","lastName":"Lovelace"}`
	var person2 ItemDataPerson
	if err := json.Unmarshal([]byte(twoFieldJSON), &person2); err != nil {
		t.Fatalf("failed to unmarshal two-field creator: %v", err)
	}
	if person2.FirstName != "Ada" || person2.LastName != "Lovelace" {
		t.Errorf("expected Ada Lovelace, got '%s' '%s'", person2.FirstName, person2.LastName)
	}
	if person2.Name != "" {
		t.Errorf("expected Name to be empty, got '%s'", person2.Name)
	}

	marshaled2, err := json.Marshal(person2)
	if err != nil {
		t.Fatalf("failed to marshal two-field creator: %v", err)
	}
	if string(marshaled2) != twoFieldJSON {
		t.Errorf("expected JSON '%s', got '%s'", twoFieldJSON, string(marshaled2))
	}
}

func TestRelationsDeserialization(t *testing.T) {
	// Empty array
	emptyArrayJSON := `{"itemType":"journalArticle","version":1,"tags":[],"relations":[],"collections":[]}`
	var itemBase1 ItemDataBase
	if err := json.Unmarshal([]byte(emptyArrayJSON), &itemBase1); err != nil {
		t.Fatalf("failed to unmarshal item with empty array relations: %v", err)
	}
	if itemBase1.Relations == nil {
		t.Errorf("expected Relations to be initialized map, got nil")
	}
	if len(itemBase1.Relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(itemBase1.Relations))
	}

	// Empty object
	emptyObjJSON := `{"itemType":"journalArticle","version":1,"tags":[],"relations":{},"collections":[]}`
	var itemBase2 ItemDataBase
	if err := json.Unmarshal([]byte(emptyObjJSON), &itemBase2); err != nil {
		t.Fatalf("failed to unmarshal item with empty object relations: %v", err)
	}
	if len(itemBase2.Relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(itemBase2.Relations))
	}

	// Relations with string list and single string
	populatedJSON := `{"itemType":"book","version":2,"tags":[],"relations":{"dc:relation":"http://example.com/1","owl:sameAs":["http://example.com/2","http://example.com/3"]},"collections":[]}`
	var itemBase3 ItemDataBase
	if err := json.Unmarshal([]byte(populatedJSON), &itemBase3); err != nil {
		t.Fatalf("failed to unmarshal item with populated relations: %v", err)
	}
	if len(itemBase3.Relations) != 2 {
		t.Fatalf("expected 2 relation keys, got %d", len(itemBase3.Relations))
	}
	dcRel, ok := itemBase3.Relations["dc:relation"]
	if !ok || len(dcRel) != 1 || dcRel[0] != "http://example.com/1" {
		t.Errorf("unexpected dc:relation value: %v", dcRel)
	}
	owlSame, ok := itemBase3.Relations["owl:sameAs"]
	if !ok || len(owlSame) != 2 || owlSame[0] != "http://example.com/2" || owlSame[1] != "http://example.com/3" {
		t.Errorf("unexpected owl:sameAs value: %v", owlSame)
	}

	// Collection RelationList empty array
	var relList RelationList
	if err := json.Unmarshal([]byte(`[]`), &relList); err != nil {
		t.Fatalf("failed to unmarshal empty array into RelationList: %v", err)
	}
	if relList == nil {
		t.Errorf("expected RelationList to be initialized, got nil")
	}
}

func TestGroupStructTags(t *testing.T) {
	groupJSON := `{
		"id": 12345,
		"version": 10,
		"name": "Test Group",
		"owner": 999,
		"type": "Private",
		"description": "Test description",
		"url": "https://example.com",
		"hasImage": 0,
		"libraryEditing": "members",
		"libraryReading": "all",
		"fileEditing": "admins",
		"admins": [999]
	}`

	var gd GroupData
	if err := json.Unmarshal([]byte(groupJSON), &gd); err != nil {
		t.Fatalf("failed to unmarshal GroupData: %v", err)
	}
	if gd.Owner != 999 {
		t.Errorf("expected owner=999, got %d", gd.Owner)
	}
	if gd.LibraryEditing != "members" {
		t.Errorf("expected libraryEditing='members', got '%s'", gd.LibraryEditing)
	}
	if gd.LibraryReading != "all" {
		t.Errorf("expected libraryReading='all', got '%s'", gd.LibraryReading)
	}
	if gd.FileEditing != "admins" {
		t.Errorf("expected fileEditing='admins', got '%s'", gd.FileEditing)
	}

	gitlabJSON := `{
		"id": 12345,
		"data": {},
		"collectionversion": 4,
		"itemversion": 5,
		"tagversion": 6
	}`
	var gg GroupGitlab
	if err := json.Unmarshal([]byte(gitlabJSON), &gg); err != nil {
		t.Fatalf("failed to unmarshal GroupGitlab: %v", err)
	}
	if gg.TagVersion != 6 {
		t.Errorf("expected tagversion=6, got %d", gg.TagVersion)
	}

	itemGitlabJSON := `{
		"libraryid": 789,
		"id": "ITEMKEY",
		"data": {},
		"meta": {}
	}`
	var ig ItemGitlab
	if err := json.Unmarshal([]byte(itemGitlabJSON), &ig); err != nil {
		t.Fatalf("failed to unmarshal ItemGitlab: %v", err)
	}
	if ig.LibraryId != 789 {
		t.Errorf("expected libraryid=789, got %d", ig.LibraryId)
	}
}

func TestCheckRetryAndBackoff(t *testing.T) {
	logger := zerolog.Nop()
	zot := &Zotero{
		Logger: &logger,
	}

	// No headers
	hEmpty := http.Header{}
	if zot.CheckRetry(hEmpty) {
		t.Errorf("CheckRetry should return false for empty headers")
	}
	if zot.CheckBackoff(hEmpty) {
		t.Errorf("CheckBackoff should return false for empty headers")
	}

	// Zero headers
	hZero := http.Header{
		"Retry-After": []string{"0"},
		"Backoff":     []string{"0"},
	}
	if zot.CheckRetry(hZero) {
		t.Errorf("CheckRetry should return false for 0 Retry-After")
	}
	if zot.CheckBackoff(hZero) {
		t.Errorf("CheckBackoff should return false for 0 Backoff")
	}

	// Invalid header strings
	hInvalid := http.Header{
		"Retry-After": []string{"invalid"},
		"Backoff":     []string{"invalid"},
	}
	if zot.CheckRetry(hInvalid) {
		t.Errorf("CheckRetry should return false for invalid Retry-After")
	}
	if zot.CheckBackoff(hInvalid) {
		t.Errorf("CheckBackoff should return false for invalid Backoff")
	}

	// Past HTTP-date Retry-After
	hPastDate := http.Header{
		"Retry-After": []string{"Fri, 31 Dec 1999 23:59:59 GMT"},
	}
	if zot.CheckRetry(hPastDate) {
		t.Errorf("CheckRetry should return false for past HTTP-Date Retry-After")
	}

	// Future HTTP-date Retry-After (1 second ahead)
	futureDate := time.Now().Add(1 * time.Second).UTC().Format(http.TimeFormat)
	hFutureDate := http.Header{
		"Retry-After": []string{futureDate},
	}
	start := time.Now()
	if !zot.CheckRetry(hFutureDate) {
		t.Errorf("CheckRetry should return true for future HTTP-Date Retry-After")
	}
	elapsed := time.Since(start)
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected sleep on future HTTP-Date Retry-After, got elapsed %v", elapsed)
	}
}

func TestTagPagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups/100/tags" {
			http.NotFound(w, r)
			return
		}
		// Verify Zotero API Version header
		if r.Header.Get("Zotero-API-Version") != "3" {
			t.Errorf("expected Zotero-API-Version: 3, got '%s'", r.Header.Get("Zotero-API-Version"))
		}

		callCount++
		start := r.URL.Query().Get("start")
		w.Header().Set("Last-Modified-Version", "42")
		w.Header().Set("Total-Results", "3")
		w.Header().Set("Content-Type", "application/json")

		if start == "0" || start == "" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"tag":"tag1","meta":{"type":0,"numItems":1}},{"tag":"tag2","meta":{"type":0,"numItems":2}}]`))
		} else if start == "100" || start == "2" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"tag":"tag3","meta":{"type":0,"numItems":3}}]`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewZotero(server.URL, "test-api-key", nil, nil, "public", false, &logger, false)
	if err != nil {
		// NewZotero calls Init() which calls getCurrentKey() -> since server is mock, we can just init client directly
	}
	burl, _ := url.Parse(server.URL)
	zot = &Zotero{
		baseUrl: burl,
		apiKey:  "test-api-key",
		Logger:  &logger,
	}
	if err := zot.Init(); err != nil {
		// getCurrentKey will fail on mock server unless mocked, but client is initialized
	}

	grp := &Group{
		Id:  100,
		Zot: zot,
	}

	tags, lastMod, err := grp.GetTagsVersionCloud(0)
	if err != nil {
		t.Fatalf("unexpected error fetching tags: %v", err)
	}
	if lastMod != 42 {
		t.Errorf("expected Last-Modified-Version=42, got %d", lastMod)
	}
	if len(*tags) != 3 {
		t.Fatalf("expected 3 tags aggregated across pages, got %d", len(*tags))
	}
	if (*tags)[0].Tag != "tag1" || (*tags)[1].Tag != "tag2" || (*tags)[2].Tag != "tag3" {
		t.Errorf("unexpected tags: %v", *tags)
	}
}

func TestApiVersionHeaderAndInit(t *testing.T) {
	receivedVersionHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedVersionHeader = r.Header.Get("Zotero-API-Version")
		if r.URL.Path == "/keys/test-api-key" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"key":"test-api-key","userId":123,"user":123,"access":{"user":{"library":true,"files":true}}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewZotero(server.URL, "test-api-key", nil, nil, "public", false, &logger, false)
	if err != nil {
		t.Fatalf("unexpected error creating Zotero client: %v", err)
	}
	if receivedVersionHeader != "3" {
		t.Errorf("expected Zotero-API-Version '3', got '%s'", receivedVersionHeader)
	}
	if zot.client.Header.Get("Zotero-API-Version") != "3" {
		t.Errorf("expected client header Zotero-API-Version to be '3', got '%s'", zot.client.Header.Get("Zotero-API-Version"))
	}
}

func TestUserAgentHeaderAndInit(t *testing.T) {
	receivedUserAgentHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgentHeader = r.Header.Get("User-Agent")
		if r.URL.Path == "/keys/test-api-key" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"key":"test-api-key","userId":123,"user":123,"access":{"user":{"library":true,"files":true}}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	logger := zerolog.Nop()
	zot, err := NewZotero(server.URL, "test-api-key", nil, nil, "public", false, &logger, false)
	if err != nil {
		t.Fatalf("unexpected error creating Zotero client: %v", err)
	}
	expectedUA := info.GetUserAgent()
	if receivedUserAgentHeader != expectedUA {
		t.Errorf("expected User-Agent '%s', got '%s'", expectedUA, receivedUserAgentHeader)
	}
	if zot.client.Header.Get("User-Agent") != expectedUA {
		t.Errorf("expected client header User-Agent to be '%s', got '%s'", expectedUA, zot.client.Header.Get("User-Agent"))
	}
}

func TestCollectionDataVersionSerialization(t *testing.T) {
	// Case 1: New collection creation (Key == "", Version == 0) -> omit key and version
	cdNew := CollectionData{
		Key:     "",
		Name:    "Test New Collection",
		Version: 0,
	}
	bytesNew, err := json.Marshal(cdNew)
	if err != nil {
		t.Fatalf("failed to marshal CollectionData: %v", err)
	}
	var mapNew map[string]any
	if err := json.Unmarshal(bytesNew, &mapNew); err != nil {
		t.Fatalf("failed to unmarshal JSON into map: %v", err)
	}
	if _, exists := mapNew["key"]; exists {
		t.Errorf("expected 'key' to be omitted when empty, but found: %v", mapNew["key"])
	}
	if _, exists := mapNew["version"]; exists {
		t.Errorf("expected 'version' to be omitted when 0, but found: %v", mapNew["version"])
	}
	if rel, exists := mapNew["relations"]; !exists {
		t.Error("expected 'relations' to be present")
	} else if relMap, ok := rel.(map[string]any); !ok || len(relMap) != 0 {
		t.Errorf("expected 'relations' to be empty object, got: %v", rel)
	}

	// Case 2: Existing collection update (Key != "", Version > 0) -> retain key and version
	cdExisting := CollectionData{
		Key:     "COLL123",
		Name:    "Test Existing Collection",
		Version: 12,
	}
	bytesExisting, err := json.Marshal(cdExisting)
	if err != nil {
		t.Fatalf("failed to marshal CollectionData: %v", err)
	}
	var mapExisting map[string]any
	if err := json.Unmarshal(bytesExisting, &mapExisting); err != nil {
		t.Fatalf("failed to unmarshal JSON into map: %v", err)
	}
	if val, exists := mapExisting["key"]; !exists || val != "COLL123" {
		t.Errorf("expected 'key' to be 'COLL123', got: %v", val)
	}
	if val, exists := mapExisting["version"]; !exists || val != float64(12) {
		t.Errorf("expected 'version' to be 12, got: %v", val)
	}
}

func TestItemGenericVersionSerialization(t *testing.T) {
	// Case 1: New item creation (Key == "", Version == 0) -> omit key and version
	itemNew := ItemGeneric{
		ItemDataBase: ItemDataBase{
			Key:      "",
			Version:  0,
			ItemType: "book",
		},
		Title: "New Book",
	}
	bytesNew, err := json.Marshal(itemNew)
	if err != nil {
		t.Fatalf("failed to marshal ItemGeneric: %v", err)
	}
	var mapNew map[string]any
	if err := json.Unmarshal(bytesNew, &mapNew); err != nil {
		t.Fatalf("failed to unmarshal JSON into map: %v", err)
	}
	if _, exists := mapNew["key"]; exists {
		t.Errorf("expected 'key' to be omitted when empty, but found: %v", mapNew["key"])
	}
	if _, exists := mapNew["version"]; exists {
		t.Errorf("expected 'version' to be omitted when 0, but found: %v", mapNew["version"])
	}
	if rel, exists := mapNew["relations"]; !exists {
		t.Error("expected 'relations' to be present")
	} else if relMap, ok := rel.(map[string]any); !ok || len(relMap) != 0 {
		t.Errorf("expected 'relations' to be empty object, got: %v", rel)
	}

	// Case 2: Existing item update (Key != "", Version > 0) -> retain key and version
	itemExisting := ItemGeneric{
		ItemDataBase: ItemDataBase{
			Key:      "ITEM123",
			Version:  45,
			ItemType: "book",
		},
		Title: "Existing Book",
	}
	bytesExisting, err := json.Marshal(itemExisting)
	if err != nil {
		t.Fatalf("failed to marshal ItemGeneric: %v", err)
	}
	var mapExisting map[string]any
	if err := json.Unmarshal(bytesExisting, &mapExisting); err != nil {
		t.Fatalf("failed to unmarshal JSON into map: %v", err)
	}
	if val, exists := mapExisting["key"]; !exists || val != "ITEM123" {
		t.Errorf("expected 'key' to be 'ITEM123', got: %v", val)
	}
	if val, exists := mapExisting["version"]; !exists || val != float64(45) {
		t.Errorf("expected 'version' to be 45, got: %v", val)
	}
}

func TestItemDataAttachmentSerialization(t *testing.T) {
	// 1. Test ItemDataAttachment struct
	att := ItemDataAttachment{
		ItemDataBase: ItemDataBase{
			Key:        "ATT12345",
			Version:    5,
			ItemType:   "attachment",
			ParentItem: "PARENT999",
			Tags: []ItemTag{
				{Tag: "pdf"},
			},
		},
		Title:       "Research Paper Attachment.pdf",
		LinkMode:    "imported_file",
		ContentType: "application/pdf",
		Charset:     "utf-8",
		Filename:    "paper.pdf",
		MD5:         "d41d8cd98f00b204e9800998ecf8427e",
		MTime:       1609459200000,
	}

	attBytes, err := json.Marshal(att)
	if err != nil {
		t.Fatalf("failed to marshal ItemDataAttachment: %v", err)
	}

	var attMap map[string]any
	if err := json.Unmarshal(attBytes, &attMap); err != nil {
		t.Fatalf("failed to unmarshal JSON into map: %v", err)
	}

	if attMap["itemType"] != "attachment" {
		t.Errorf("expected itemType 'attachment', got: %v", attMap["itemType"])
	}
	if attMap["linkMode"] != "imported_file" {
		t.Errorf("expected linkMode 'imported_file', got: %v", attMap["linkMode"])
	}
	if attMap["contentType"] != "application/pdf" {
		t.Errorf("expected contentType 'application/pdf', got: %v", attMap["contentType"])
	}
	if attMap["filename"] != "paper.pdf" {
		t.Errorf("expected filename 'paper.pdf', got: %v", attMap["filename"])
	}
	if attMap["parentItem"] != "PARENT999" {
		t.Errorf("expected parentItem 'PARENT999', got: %v", attMap["parentItem"])
	}
	if attMap["md5"] != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("expected md5 'd41d8cd98f00b204e9800998ecf8427e', got: %v", attMap["md5"])
	}

	var attUnmarshaled ItemDataAttachment
	if err := json.Unmarshal(attBytes, &attUnmarshaled); err != nil {
		t.Fatalf("failed to unmarshal ItemDataAttachment: %v", err)
	}
	if attUnmarshaled.Filename != att.Filename || attUnmarshaled.ParentItem != att.ParentItem || attUnmarshaled.LinkMode != att.LinkMode {
		t.Errorf("unmarshaled ItemDataAttachment mismatch: %+v vs %+v", attUnmarshaled, att)
	}

	// 2. Test ItemGeneric with attachment fields
	genAtt := ItemGeneric{
		ItemDataBase: ItemDataBase{
			Key:        "ATTGEN12",
			Version:    2,
			ItemType:   "attachment",
			ParentItem: "PARENT888",
		},
		Title:       "Document Note.txt",
		LinkMode:    "imported_file",
		ContentType: "text/plain",
		Filename:    "document.txt",
		MD5:         "098f6bcd4621d373cade4e832627b4f6",
		MTime:       1620000000000,
	}

	genBytes, err := json.Marshal(genAtt)
	if err != nil {
		t.Fatalf("failed to marshal ItemGeneric attachment: %v", err)
	}

	var genUnmarshaled ItemGeneric
	if err := json.Unmarshal(genBytes, &genUnmarshaled); err != nil {
		t.Fatalf("failed to unmarshal ItemGeneric attachment: %v", err)
	}
	if genUnmarshaled.ItemType != "attachment" || genUnmarshaled.LinkMode != "imported_file" || genUnmarshaled.ContentType != "text/plain" || genUnmarshaled.Filename != "document.txt" || genUnmarshaled.ParentItem != "PARENT888" {
		t.Errorf("unmarshaled ItemGeneric attachment mismatch: %+v vs %+v", genUnmarshaled, genAtt)
	}
}
