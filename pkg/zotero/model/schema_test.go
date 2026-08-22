package model_test

import (
	"testing"

	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func TestSchemaLoad(t *testing.T) {
	s := model.GetSchema()
	if s == nil {
		t.Fatal("expected non-nil schema")
	}
	if s.Version <= 0 {
		t.Fatalf("expected positive schema version, got %d", s.Version)
	}
	if len(s.ItemTypes) == 0 {
		t.Fatal("expected item types in schema")
	}
}

func TestSchemaItemTypes(t *testing.T) {
	validTypes := []string{"book", "journalArticle", "webpage", "attachment", "note", "artwork", "thesis"}
	for _, it := range validTypes {
		if !model.IsValidItemType(it) {
			t.Errorf("expected itemType %q to be valid", it)
		}
	}

	invalidTypes := []string{"nonexistentType", "foo", "bar"}
	for _, it := range invalidTypes {
		if model.IsValidItemType(it) {
			t.Errorf("expected itemType %q to be invalid", it)
		}
	}
}

func TestSchemaFields(t *testing.T) {
	if !model.IsValidField("book", "title") {
		t.Error("expected 'title' to be valid for book")
	}
	if !model.IsValidField("book", "ISBN") {
		t.Error("expected 'ISBN' to be valid for book")
	}
	if model.IsValidField("book", "runningTime") {
		t.Error("expected 'runningTime' to be invalid for book")
	}

	if !model.IsValidField("journalArticle", "publicationTitle") {
		t.Error("expected 'publicationTitle' to be valid for journalArticle")
	}
	if !model.IsValidField("attachment", "linkMode") {
		t.Error("expected 'linkMode' to be valid for attachment")
	}
	if !model.IsValidField("note", "note") {
		t.Error("expected 'note' to be valid for note")
	}

	fields := model.GetValidFields("book")
	if len(fields) == 0 {
		t.Error("expected non-empty fields list for book")
	}
}

func TestSchemaCreatorTypes(t *testing.T) {
	if !model.IsValidCreatorType("book", "author") {
		t.Error("expected 'author' to be valid creatorType for book")
	}
	if !model.IsValidCreatorType("book", "editor") {
		t.Error("expected 'editor' to be valid creatorType for book")
	}
	if model.IsValidCreatorType("book", "programmer") {
		t.Error("expected 'programmer' to be invalid creatorType for book")
	}

	creatorTypes := model.GetValidCreatorTypes("book")
	if len(creatorTypes) == 0 {
		t.Error("expected non-empty creator types for book")
	}
}
