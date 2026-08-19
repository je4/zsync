package zotero

import (
	"encoding/json"
	"github.com/rs/zerolog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
