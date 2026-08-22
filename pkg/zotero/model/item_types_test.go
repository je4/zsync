package model_test

import (
	"encoding/json/v2"
	"testing"

	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func TestItemDataInterface(t *testing.T) {
	var _ model.ItemData = (*model.ItemBook)(nil)
	var _ model.ItemData = (*model.ItemJournalArticle)(nil)
	var _ model.ItemData = (*model.ItemFilm)(nil)
	var _ model.ItemData = (*model.ItemAttachment)(nil)
	var _ model.ItemData = (*model.ItemNote)(nil)
	var _ model.ItemData = (*model.ItemGeneric)(nil)
}

func TestItemBookToAndFromGeneric(t *testing.T) {
	book := model.ItemBook{
		ItemDataBase: model.ItemDataBase{
			Key:      "BOOK123",
			Version:  5,
			ItemType: "book",
			Creators: []model.ItemDataPerson{
				{CreatorType: "author", FirstName: "Donald", LastName: "Knuth"},
			},
			Tags: []model.ItemTag{
				{Tag: "algorithms"},
			},
		},
		Title:        "The Art of Computer Programming",
		AbstractNote: "Fundamental Algorithms",
		Series:       "TAOCP",
		SeriesNumber: "1",
		Volume:       "1",
		Edition:      "3rd",
		Place:        "Boston",
		Publisher:    "Addison-Wesley",
		Date:         "1997",
		NumPages:     "650",
		Language:     "en",
		ISBN:         "978-0201896831",
		ShortTitle:   "TAOCP Vol 1",
		Url:          "https://www-cs-faculty.stanford.edu/~knuth/taocp.html",
	}

	if book.GetItemType() != "book" {
		t.Fatalf("expected itemType 'book', got %q", book.GetItemType())
	}

	// Test ToGeneric()
	gen := book.ToGeneric()
	if gen == nil {
		t.Fatal("expected non-nil ItemGeneric")
	}
	if gen.Key != "BOOK123" || gen.Version != 5 || gen.ItemType != "book" {
		t.Fatalf("unexpected base fields in generic: %+v", gen.ItemDataBase)
	}
	if gen.Title != "The Art of Computer Programming" {
		t.Errorf("expected Title to match, got %q", gen.Title)
	}
	if gen.ISBN() != "978-0201896831" {
		t.Errorf("expected ISBN %q, got %q", "978-0201896831", gen.ISBN())
	}
	if gen.Publisher() != "Addison-Wesley" {
		t.Errorf("expected Publisher %q, got %q", "Addison-Wesley", gen.Publisher())
	}
	if gen.NumPages() != "650" {
		t.Errorf("expected NumPages %q, got %q", "650", gen.NumPages())
	}

	// Test FromGeneric()
	var restored model.ItemBook
	if err := restored.FromGeneric(gen); err != nil {
		t.Fatalf("FromGeneric failed: %v", err)
	}

	if restored.Key != book.Key || restored.Version != book.Version {
		t.Errorf("base fields mismatch: got %+v, want %+v", restored.ItemDataBase, book.ItemDataBase)
	}
	if restored.Title != book.Title || restored.ISBN != book.ISBN || restored.Publisher != book.Publisher {
		t.Errorf("restored fields mismatch: got %+v, want %+v", restored, book)
	}
}

func TestItemJournalArticleToAndFromGeneric(t *testing.T) {
	article := model.ItemJournalArticle{
		ItemDataBase: model.ItemDataBase{
			Key:      "ART456",
			Version:  1,
			ItemType: "journalArticle",
		},
		Title:            "Attention Is All You Need",
		AbstractNote:     "The dominant sequence transduction models are based on complex recurrent or convolutional neural networks...",
		PublicationTitle: "Advances in Neural Information Processing Systems",
		Volume:           "30",
		Pages:            "5998-6008",
		Date:             "2017",
		DOI:              "10.5555/3295222.3295349",
		ISSN:             "1049-5258",
		Url:              "https://arxiv.org/abs/1706.03762",
	}

	gen := article.ToGeneric()
	if gen.DOI() != "10.5555/3295222.3295349" {
		t.Errorf("expected DOI %q, got %q", article.DOI, gen.DOI())
	}
	if gen.PublicationTitle() != "Advances in Neural Information Processing Systems" {
		t.Errorf("expected PublicationTitle %q, got %q", article.PublicationTitle, gen.PublicationTitle())
	}
	if gen.ISSN() != "1049-5258" {
		t.Errorf("expected ISSN %q, got %q", article.ISSN, gen.ISSN())
	}

	var restored model.ItemJournalArticle
	if err := restored.FromGeneric(gen); err != nil {
		t.Fatalf("FromGeneric failed: %v", err)
	}
	if restored.DOI != article.DOI || restored.PublicationTitle != article.PublicationTitle || restored.Pages != article.Pages {
		t.Errorf("restored mismatch: got %+v, want %+v", restored, article)
	}
}

func TestItemFilmToAndFromGeneric(t *testing.T) {
	film := model.ItemFilm{
		ItemDataBase: model.ItemDataBase{
			Key:      "FILM789",
			Version:  2,
			ItemType: "film",
		},
		Title:       "Inception",
		Distributor: "Warner Bros. Pictures",
		Date:        "2010",
		RunningTime: "148 min",
		Genre:       "Sci-Fi",
	}

	gen := film.ToGeneric()
	if gen.RunningTime() != "148 min" {
		t.Errorf("expected RunningTime %q, got %q", film.RunningTime, gen.RunningTime())
	}
	if gen.Genre() != "Sci-Fi" {
		t.Errorf("expected Genre %q, got %q", film.Genre, gen.Genre())
	}
	if gen.Distributor() != "Warner Bros. Pictures" {
		t.Errorf("expected Distributor %q, got %q", film.Distributor, gen.Distributor())
	}

	var restored model.ItemFilm
	if err := restored.FromGeneric(gen); err != nil {
		t.Fatalf("FromGeneric failed: %v", err)
	}
	if restored.RunningTime != film.RunningTime || restored.Genre != film.Genre || restored.Distributor != film.Distributor {
		t.Errorf("restored film mismatch: got %+v, want %+v", restored, film)
	}
}

func TestItemAttachmentAndNote(t *testing.T) {
	att := model.ItemAttachment{
		ItemDataBase: model.ItemDataBase{
			Key:      "ATT101",
			ItemType: "attachment",
		},
		Title:       "Document PDF",
		LinkMode:    "imported_file",
		ContentType: "application/pdf",
		Filename:    "paper.pdf",
		MD5:         "d41d8cd98f00b204e9800998ecf8427e",
		MTime:       1629500000000,
	}

	gen := att.ToGeneric()
	if gen.Filename != "paper.pdf" || gen.MD5 != "d41d8cd98f00b204e9800998ecf8427e" || gen.MTime != 1629500000000 {
		t.Errorf("unexpected attachment fields in generic: %+v", gen)
	}

	var restoredAtt model.ItemAttachment
	if err := restoredAtt.FromGeneric(gen); err != nil {
		t.Fatalf("FromGeneric failed: %v", err)
	}
	if restoredAtt.MTime != att.MTime || restoredAtt.Filename != att.Filename || restoredAtt.MD5 != att.MD5 {
		t.Errorf("restored attachment mismatch: got %+v, want %+v", restoredAtt, att)
	}

	note := model.ItemNote{
		ItemDataBase: model.ItemDataBase{
			Key:      "NOTE102",
			ItemType: "note",
		},
		Note: "<p>Research notes on transformer architecture</p>",
	}
	noteGen := note.ToGeneric()
	if noteGen.Note != "<p>Research notes on transformer architecture</p>" {
		t.Errorf("expected note content in generic, got %q", noteGen.Note)
	}
	var restoredNote model.ItemNote
	if err := restoredNote.FromGeneric(noteGen); err != nil {
		t.Fatalf("FromGeneric failed for note: %v", err)
	}
	if restoredNote.Note != note.Note {
		t.Errorf("restored note mismatch: got %q, want %q", restoredNote.Note, note.Note)
	}
}

func TestNilSafety(t *testing.T) {
	var nilBook *model.ItemBook
	if nilBook.ToGeneric() != nil {
		t.Error("expected nil ItemGeneric from nil ItemBook")
	}

	var book model.ItemBook
	if err := book.FromGeneric(nil); err == nil {
		t.Error("expected error when populating from nil ItemGeneric")
	}
}

func TestConcreteStructJSONRoundtrip(t *testing.T) {
	book := model.ItemBook{
		ItemDataBase: model.ItemDataBase{
			Key:      "JSON1",
			Version:  10,
			ItemType: "book",
		},
		Title:     "Structure and Interpretation of Computer Programs",
		Publisher: "MIT Press",
		ISBN:      "978-0262510875",
		Date:      "1996",
	}

	bookBytes, err := json.Marshal(book)
	if err != nil {
		t.Fatalf("failed to marshal ItemBook: %v", err)
	}

	var gen model.ItemGeneric
	if err := json.Unmarshal(bookBytes, &gen); err != nil {
		t.Fatalf("failed to unmarshal JSON into ItemGeneric: %v", err)
	}

	if gen.Title != book.Title {
		t.Errorf("expected Title %q, got %q", book.Title, gen.Title)
	}
	if gen.ISBN() != book.ISBN {
		t.Errorf("expected ISBN %q, got %q", book.ISBN, gen.ISBN())
	}
	if gen.Publisher() != book.Publisher {
		t.Errorf("expected Publisher %q, got %q", book.Publisher, gen.Publisher())
	}

	// Marshal ItemGeneric and unmarshal back into ItemBook
	genBytes, err := json.Marshal(gen)
	if err != nil {
		t.Fatalf("failed to marshal ItemGeneric: %v", err)
	}

	var book2 model.ItemBook
	if err := json.Unmarshal(genBytes, &book2); err != nil {
		t.Fatalf("failed to unmarshal JSON into ItemBook: %v", err)
	}

	if book2.Title != book.Title || book2.ISBN != book.ISBN || book2.Publisher != book.Publisher {
		t.Errorf("JSON roundtrip mismatch: got %+v, want %+v", book2, book)
	}
}

func TestEmptyFieldsHandling(t *testing.T) {
	// A book with only some fields populated
	book := model.ItemBook{
		ItemDataBase: model.ItemDataBase{
			Key:      "BOOK_EMPTY",
			Version:  1,
			ItemType: "book",
		},
		Title: "Minimal Book",
	}

	gen := book.ToGeneric()
	if gen == nil {
		t.Fatal("expected non-nil ItemGeneric")
	}

	// ExtraFields should not have entries for empty string fields
	if len(gen.ExtraFields) != 0 {
		t.Errorf("expected empty ExtraFields, got %+v", gen.ExtraFields)
	}

	// Dynamic getters should return empty string
	if gen.ISBN() != "" {
		t.Errorf("expected empty ISBN, got %q", gen.ISBN())
	}
	if gen.Publisher() != "" {
		t.Errorf("expected empty Publisher, got %q", gen.Publisher())
	}

	// Validation must succeed
	if err := gen.Validate(); err != nil {
		t.Errorf("expected validation to pass, got: %v", err)
	}

	// Setting a value and then resetting to empty string cleans up ExtraFields
	gen.SetISBN("978-1234567890")
	if gen.ISBN() != "978-1234567890" {
		t.Errorf("expected ISBN to be set, got %q", gen.ISBN())
	}
	if len(gen.ExtraFields) != 1 {
		t.Errorf("expected 1 entry in ExtraFields, got %d", len(gen.ExtraFields))
	}

	gen.SetISBN("")
	if gen.ISBN() != "" {
		t.Errorf("expected ISBN to be empty, got %q", gen.ISBN())
	}
	if len(gen.ExtraFields) != 0 {
		t.Errorf("expected ExtraFields to be empty after clearing ISBN, got %+v", gen.ExtraFields)
	}
}
