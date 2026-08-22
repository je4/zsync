package model

import (
	"encoding/json/v2"
	"reflect"
	"strings"
	"testing"
)

func assertJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()

	var expectedValue, actualValue any
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatalf("failed to parse expected JSON: %v", err)
	}
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("failed to parse actual JSON: %v", err)
	}
	if !reflect.DeepEqual(expectedValue, actualValue) {
		t.Errorf("expected JSON %s, got %s", expected, actual)
	}
}

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
	assertJSONEqual(t, []byte(singleFieldJSON), marshaled1)

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
	assertJSONEqual(t, []byte(twoFieldJSON), marshaled2)
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
	if parent, exists := mapNew["parentCollection"]; !exists {
		t.Error("expected 'parentCollection' to be present")
	} else if parentBool, ok := parent.(bool); !ok || parentBool != false {
		t.Errorf("expected 'parentCollection' to be false, got: %v", parent)
	}

	// Case 2: Existing collection update (Key != "", Version > 0) -> include key and version
	cdExisting := CollectionData{
		Key:              "COLL123",
		Name:             "Existing Collection",
		Version:          45,
		ParentCollection: "PARENT456",
		Relations: RelationList{
			"owl:sameAs": "http://example.com/same",
		},
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
	if val, exists := mapExisting["version"]; !exists || val != float64(45) {
		t.Errorf("expected 'version' to be 45, got: %v", val)
	}
}

func TestItemGenericVersionSerialization(t *testing.T) {
	// Case 1: New item creation (Key == "", Version == 0) -> omit key and version
	itemNew := ItemGeneric{
		Key:      "",
		Version:  0,
		ItemType: "book",
		Title:    "New Book",
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
		Key:      "ITEM123",
		Version:  45,
		ItemType: "book",
		Title:    "Existing Book",
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
	// 1. Test the generated ItemAttachment struct
	att := ItemAttachment{
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
		t.Fatalf("failed to marshal ItemAttachment: %v", err)
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

	var attUnmarshaled ItemAttachment
	if err := json.Unmarshal(attBytes, &attUnmarshaled); err != nil {
		t.Fatalf("failed to unmarshal ItemAttachment: %v", err)
	}
	if attUnmarshaled.Filename != att.Filename || attUnmarshaled.ParentItem != att.ParentItem || attUnmarshaled.LinkMode != att.LinkMode {
		t.Errorf("unmarshaled ItemAttachment mismatch: %+v vs %+v", attUnmarshaled, att)
	}

	// 2. Test ItemGeneric with attachment fields
	genAtt := ItemGeneric{
		Key:         "ATTGEN12",
		Version:     2,
		ItemType:    "attachment",
		ParentItem:  "PARENT888",
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

func TestText2Metadata(t *testing.T) {
	desc := "some text tag:value key:\"spaced value\" foo:bar"
	meta := Text2Metadata(desc)
	if len(meta["tag"]) != 1 || meta["tag"][0] != "value" {
		t.Errorf("expected tag=value, got %v", meta["tag"])
	}
	if len(meta["key"]) != 1 || meta["key"][0] != "spaced value" {
		t.Errorf("expected key='spaced value', got %v", meta["key"])
	}
	if len(meta["foo"]) != 1 || meta["foo"][0] != "bar" {
		t.Errorf("expected foo=bar, got %v", meta["foo"])
	}

	noMeta := strings.TrimSpace(TextNoMeta(desc))
	if noMeta != "some text" {
		t.Errorf("expected 'some text', got '%s'", noMeta)
	}
}

func TestItemGenericGetSetDelete(t *testing.T) {
	var item ItemGeneric

	// Test core field direct & dynamic access
	item.Title = "Original Title"
	if item.GetString("title") != "Original Title" {
		t.Errorf("expected 'Original Title', got '%s'", item.GetString("title"))
	}

	item.SetString("title", "New Title")
	if item.Title != "New Title" {
		t.Errorf("expected 'New Title', got '%s'", item.Title)
	}
	if item.GetString("title") != "New Title" {
		t.Errorf("expected 'New Title', got '%s'", item.GetString("title"))
	}

	// Test dynamic ExtraFields access
	item.SetString("publisher", "Springer")
	item.SetString("DOI", "10.1000/182")
	item.Set("numPages", "350")

	if item.GetString("publisher") != "Springer" {
		t.Errorf("expected 'Springer', got '%s'", item.GetString("publisher"))
	}
	if item.GetString("DOI") != "10.1000/182" {
		t.Errorf("expected '10.1000/182', got '%s'", item.GetString("DOI"))
	}
	val, ok := item.Get("numPages")
	if !ok || val != "350" {
		t.Errorf("expected '350', got %v (ok=%v)", val, ok)
	}

	// Test deletion
	item.Delete("publisher")
	if _, ok := item.Get("publisher"); ok {
		t.Errorf("expected 'publisher' to be deleted")
	}
	item.Delete("title")
	if item.Title != "" {
		t.Errorf("expected Title to be empty after Delete, got '%s'", item.Title)
	}
}

func TestItemGenericJSONRoundtripDynamicFields(t *testing.T) {
	inputJSON := `{
		"key": "ITEM456",
		"version": 12,
		"itemType": "book",
		"title": "Clean Code",
		"abstractNote": "A handbook of agile software craftsmanship",
		"date": "2008",
		"publisher": "Prentice Hall",
		"numPages": "464",
		"ISBN": "978-0132350884",
		"creators": [
			{"creatorType":"author","firstName":"Robert C.","lastName":"Martin"}
		],
		"tags": [
			{"tag":"programming"}
		],
		"relations": {}
	}`

	var item ItemGeneric
	if err := json.Unmarshal([]byte(inputJSON), &item); err != nil {
		t.Fatalf("failed to unmarshal ItemGeneric: %v", err)
	}

	// Verify core fields
	if item.Key != "ITEM456" || item.Version != 12 || item.ItemType != "book" || item.Title != "Clean Code" {
		t.Fatalf("unexpected core fields: %+v", item)
	}

	// Verify dynamic fields
	if item.GetString("publisher") != "Prentice Hall" {
		t.Errorf("expected publisher 'Prentice Hall', got '%s'", item.GetString("publisher"))
	}
	if item.GetString("numPages") != "464" {
		t.Errorf("expected numPages '464', got '%s'", item.GetString("numPages"))
	}
	if item.GetString("ISBN") != "978-0132350884" {
		t.Errorf("expected ISBN '978-0132350884', got '%s'", item.GetString("ISBN"))
	}

	// Verify creators and tags
	if len(item.Creators) != 1 || item.Creators[0].LastName != "Martin" {
		t.Errorf("unexpected creators: %+v", item.Creators)
	}
	if len(item.Tags) != 1 || item.Tags[0].Tag != "programming" {
		t.Errorf("unexpected tags: %+v", item.Tags)
	}

	// Re-marshal to JSON
	outputBytes, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal ItemGeneric: %v", err)
	}

	var outputMap map[string]any
	if err := json.Unmarshal(outputBytes, &outputMap); err != nil {
		t.Fatalf("failed to unmarshal output JSON: %v", err)
	}

	if outputMap["publisher"] != "Prentice Hall" {
		t.Errorf("expected publisher 'Prentice Hall' in marshaled JSON, got %v", outputMap["publisher"])
	}
	if outputMap["numPages"] != "464" {
		t.Errorf("expected numPages '464' in marshaled JSON, got %v", outputMap["numPages"])
	}
	if outputMap["ISBN"] != "978-0132350884" {
		t.Errorf("expected ISBN in marshaled JSON, got %v", outputMap["ISBN"])
	}
	if outputMap["title"] != "Clean Code" {
		t.Errorf("expected title in marshaled JSON, got %v", outputMap["title"])
	}
}

func TestItemGenericValidation(t *testing.T) {
	// Valid book
	validBook := ItemGeneric{
		ItemDataBase: ItemDataBase{
			ItemType: "book",
			Creators: []ItemDataPerson{
				{CreatorType: "author", LastName: "Martin"},
			},
		},
		Title: "Clean Architecture",
	}
	validBook.SetString("publisher", "Prentice Hall")
	validBook.SetString("ISBN", "9780134494166")

	if err := validBook.Validate(); err != nil {
		t.Errorf("expected valid book to pass validation, got: %v", err)
	}

	// Invalid itemType
	invalidType := ItemGeneric{
		ItemDataBase: ItemDataBase{
			ItemType: "unknownItemTypeXYZ",
		},
	}
	if err := invalidType.Validate(); err == nil {
		t.Error("expected invalid itemType to fail validation")
	}

	// Invalid field for itemType
	invalidField := ItemGeneric{
		ItemDataBase: ItemDataBase{
			ItemType: "book",
		},
		Title: "Book with invalid field",
	}
	invalidField.SetString("runningTime", "120 mins") // runningTime is not valid for book
	if err := invalidField.Validate(); err == nil {
		t.Error("expected invalid field for book to fail validation")
	}

	// Invalid creatorType for itemType
	invalidCreator := ItemGeneric{
		ItemDataBase: ItemDataBase{
			ItemType: "book",
			Creators: []ItemDataPerson{
				{CreatorType: "programmer", LastName: "Dev"},
			},
		},
		Title: "Book with invalid creator",
	}
	if err := invalidCreator.Validate(); err == nil {
		t.Error("expected invalid creatorType for book to fail validation")
	}
}
