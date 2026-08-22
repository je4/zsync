package model

import (
	"encoding/json/v2"
	"fmt"
	"strconv"

	"emperror.dev/errors"
)

// ItemGeneric stores the common Zotero item fields and arbitrary item-type
// fields while preserving the original JSON representation.
type ItemGeneric struct {
	ItemDataBase

	// Core common fields
	Title        string `json:"title,omitempty"`
	AbstractNote string `json:"abstractNote,omitempty"`
	Date         string `json:"date,omitempty"`
	Url          string `json:"url,omitempty"`
	Extra        string `json:"extra,omitempty"`
	ShortTitle   string `json:"shortTitle,omitempty"`

	// Attachment fields
	LinkMode    string `json:"linkMode,omitempty"`
	Note        string `json:"note,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Charset     string `json:"charset,omitempty"`
	Filename    string `json:"filename,omitempty"`
	MD5         string `json:"md5,omitempty"`
	MTime       int64  `json:"mtime,omitzero"`

	// Dynamic schema fields
	ExtraFields map[string]string `json:"-"`
}

type itemGenericKnown struct {
	ItemDataBase

	Title        string `json:"title,omitempty"`
	AbstractNote string `json:"abstractNote,omitempty"`
	Date         string `json:"date,omitempty"`
	Url          string `json:"url,omitempty"`
	Extra        string `json:"extra,omitempty"`
	ShortTitle   string `json:"shortTitle,omitempty"`

	LinkMode    string `json:"linkMode,omitempty"`
	Note        string `json:"note,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Charset     string `json:"charset,omitempty"`
	Filename    string `json:"filename,omitempty"`
	MD5         string `json:"md5,omitempty"`
	MTime       int64  `json:"mtime,omitzero"`
}

func (item ItemGeneric) MarshalJSON() ([]byte, error) {
	known := itemGenericKnown{
		ItemDataBase: item.ItemDataBase,
		Title:        item.Title,
		AbstractNote: item.AbstractNote,
		Date:         item.Date,
		Url:          item.Url,
		Extra:        item.Extra,
		ShortTitle:   item.ShortTitle,
		LinkMode:     item.LinkMode,
		Note:         item.Note,
		ContentType:  item.ContentType,
		Charset:      item.Charset,
		Filename:     item.Filename,
		MD5:          item.MD5,
		MTime:        item.MTime,
	}

	if len(item.ExtraFields) == 0 {
		return json.Marshal(known)
	}

	knownBytes, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := json.Unmarshal(knownBytes, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = make(map[string]any)
	}

	for k, v := range item.ExtraFields {
		if v != "" {
			m[k] = v
		}
	}

	return json.Marshal(m)
}

func (item *ItemGeneric) UnmarshalJSON(data []byte) error {
	var known itemGenericKnown
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}

	item.ItemDataBase = known.ItemDataBase
	item.Title = known.Title
	item.AbstractNote = known.AbstractNote
	item.Date = known.Date
	item.Url = known.Url
	item.Extra = known.Extra
	item.ShortTitle = known.ShortTitle
	item.LinkMode = known.LinkMode
	item.Note = known.Note
	item.ContentType = known.ContentType
	item.Charset = known.Charset
	item.Filename = known.Filename
	item.MD5 = known.MD5
	item.MTime = known.MTime

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	delete(raw, "key")
	delete(raw, "version")
	delete(raw, "itemType")
	delete(raw, "tags")
	delete(raw, "relations")
	delete(raw, "parentItem")
	delete(raw, "collections")
	delete(raw, "dateAdded")
	delete(raw, "dateModified")
	delete(raw, "creators")

	delete(raw, "title")
	delete(raw, "abstractNote")
	delete(raw, "date")
	delete(raw, "url")
	delete(raw, "extra")
	delete(raw, "shortTitle")

	delete(raw, "linkMode")
	delete(raw, "note")
	delete(raw, "contentType")
	delete(raw, "charset")
	delete(raw, "filename")
	delete(raw, "md5")
	delete(raw, "mtime")

	if len(raw) > 0 {
		extra := make(map[string]string, len(raw))
		for k, v := range raw {
			if v == nil {
				continue
			}
			if s, ok := v.(string); ok {
				if s != "" {
					extra[k] = s
				}
			} else {
				extra[k] = fmt.Sprintf("%v", v)
			}
		}
		if len(extra) > 0 {
			item.ExtraFields = extra
		} else {
			item.ExtraFields = nil
		}
	} else {
		item.ExtraFields = nil
	}

	return nil
}

// GetItemType returns the Zotero item type.
func (item *ItemGeneric) GetItemType() string {
	return item.ItemType
}

func (item *ItemGeneric) ToGeneric() *ItemGeneric {
	return item
}

func (item *ItemGeneric) FromGeneric(gen *ItemGeneric) error {
	if gen == nil {
		return errors.New("cannot populate from nil ItemGeneric")
	}
	*item = *gen
	return nil
}

// Get returns a field value and whether the field is present.
func (item *ItemGeneric) Get(field string) (string, bool) {
	switch field {
	case "key":
		return item.Key, item.Key != ""
	case "version":
		return strconv.FormatInt(item.Version, 10), item.Version != 0
	case "itemType":
		return item.ItemType, item.ItemType != ""
	case "parentItem":
		return item.ParentItem, item.ParentItem != ""
	case "dateAdded":
		return item.DateAdded, item.DateAdded != ""
	case "dateModified":
		return item.DateModified, item.DateModified != ""
	case "title":
		return item.Title, item.Title != ""
	case "abstractNote":
		return item.AbstractNote, item.AbstractNote != ""
	case "date":
		return item.Date, item.Date != ""
	case "url":
		return item.Url, item.Url != ""
	case "extra":
		return item.Extra, item.Extra != ""
	case "shortTitle":
		return item.ShortTitle, item.ShortTitle != ""
	case "linkMode":
		return item.LinkMode, item.LinkMode != ""
	case "note":
		return item.Note, item.Note != ""
	case "contentType":
		return item.ContentType, item.ContentType != ""
	case "charset":
		return item.Charset, item.Charset != ""
	case "filename":
		return item.Filename, item.Filename != ""
	case "md5":
		return item.MD5, item.MD5 != ""
	case "mtime":
		return strconv.FormatInt(item.MTime, 10), item.MTime != 0
	default:
		if item.ExtraFields == nil {
			return "", false
		}
		val, ok := item.ExtraFields[field]
		return val, ok
	}
}

func (item *ItemGeneric) GetString(field string) string {
	val, _ := item.Get(field)
	return val
}

// Set assigns a raw field value.
func (item *ItemGeneric) Set(field, val string) {
	switch field {
	case "key":
		item.Key = val
	case "version":
		v, _ := strconv.ParseInt(val, 10, 64)
		item.Version = v
	case "itemType":
		item.ItemType = val
	case "parentItem":
		item.ParentItem = val
	case "dateAdded":
		item.DateAdded = val
	case "dateModified":
		item.DateModified = val
	case "title":
		item.Title = val
	case "abstractNote":
		item.AbstractNote = val
	case "date":
		item.Date = val
	case "url":
		item.Url = val
	case "extra":
		item.Extra = val
	case "shortTitle":
		item.ShortTitle = val
	case "linkMode":
		item.LinkMode = val
	case "note":
		item.Note = val
	case "contentType":
		item.ContentType = val
	case "charset":
		item.Charset = val
	case "filename":
		item.Filename = val
	case "md5":
		item.MD5 = val
	case "mtime":
		v, _ := strconv.ParseInt(val, 10, 64)
		item.MTime = v
	default:
		if val == "" {
			delete(item.ExtraFields, field)
		} else {
			if item.ExtraFields == nil {
				item.ExtraFields = make(map[string]string)
			}
			item.ExtraFields[field] = val
		}
	}
}

func (item *ItemGeneric) SetString(field, val string) {
	item.Set(field, val)
}

// Delete removes a field from the item.
func (item *ItemGeneric) Delete(field string) {
	switch field {
	case "key":
		item.Key = ""
	case "version":
		item.Version = 0
	case "itemType":
		item.ItemType = ""
	case "parentItem":
		item.ParentItem = ""
	case "dateAdded":
		item.DateAdded = ""
	case "dateModified":
		item.DateModified = ""
	case "title":
		item.Title = ""
	case "abstractNote":
		item.AbstractNote = ""
	case "date":
		item.Date = ""
	case "url":
		item.Url = ""
	case "extra":
		item.Extra = ""
	case "shortTitle":
		item.ShortTitle = ""
	case "linkMode":
		item.LinkMode = ""
	case "note":
		item.Note = ""
	case "contentType":
		item.ContentType = ""
	case "charset":
		item.Charset = ""
	case "filename":
		item.Filename = ""
	case "md5":
		item.MD5 = ""
	case "mtime":
		item.MTime = 0
	case "tags":
		item.Tags = nil
	case "creators":
		item.Creators = nil
	case "collections":
		item.Collections = nil
	case "relations":
		item.Relations = nil
	default:
		if item.ExtraFields != nil {
			delete(item.ExtraFields, field)
		}
	}
}

// Validate checks the item against the embedded Zotero schema.
func (item *ItemGeneric) Validate() error {
	if item.ItemType == "" {
		return fmt.Errorf("itemType is required")
	}
	schema := GetSchema()
	if !schema.IsValidItemType(item.ItemType) {
		return fmt.Errorf("invalid itemType %q", item.ItemType)
	}

	for _, c := range item.Creators {
		if c.CreatorType != "" && !schema.IsValidCreatorType(item.ItemType, c.CreatorType) {
			return fmt.Errorf("creatorType %q is not valid for itemType %q", c.CreatorType, item.ItemType)
		}
	}

	if item.Title != "" && !schema.IsValidField(item.ItemType, "title") {
		return fmt.Errorf("field 'title' is not valid for itemType %q", item.ItemType)
	}
	if item.AbstractNote != "" && !schema.IsValidField(item.ItemType, "abstractNote") {
		return fmt.Errorf("field 'abstractNote' is not valid for itemType %q", item.ItemType)
	}
	if item.Date != "" && !schema.IsValidField(item.ItemType, "date") {
		return fmt.Errorf("field 'date' is not valid for itemType %q", item.ItemType)
	}
	if item.Url != "" && !schema.IsValidField(item.ItemType, "url") {
		return fmt.Errorf("field 'url' is not valid for itemType %q", item.ItemType)
	}
	if item.Extra != "" && !schema.IsValidField(item.ItemType, "extra") {
		return fmt.Errorf("field 'extra' is not valid for itemType %q", item.ItemType)
	}
	if item.ShortTitle != "" && !schema.IsValidField(item.ItemType, "shortTitle") {
		return fmt.Errorf("field 'shortTitle' is not valid for itemType %q", item.ItemType)
	}
	if item.LinkMode != "" && !schema.IsValidField(item.ItemType, "linkMode") {
		return fmt.Errorf("field 'linkMode' is not valid for itemType %q", item.ItemType)
	}
	if item.Note != "" && !schema.IsValidField(item.ItemType, "note") {
		return fmt.Errorf("field 'note' is not valid for itemType %q", item.ItemType)
	}
	if item.ContentType != "" && !schema.IsValidField(item.ItemType, "contentType") {
		return fmt.Errorf("field 'contentType' is not valid for itemType %q", item.ItemType)
	}
	if item.Charset != "" && !schema.IsValidField(item.ItemType, "charset") {
		return fmt.Errorf("field 'charset' is not valid for itemType %q", item.ItemType)
	}
	if item.Filename != "" && !schema.IsValidField(item.ItemType, "filename") {
		return fmt.Errorf("field 'filename' is not valid for itemType %q", item.ItemType)
	}
	if item.MD5 != "" && !schema.IsValidField(item.ItemType, "md5") {
		return fmt.Errorf("field 'md5' is not valid for itemType %q", item.ItemType)
	}
	if item.MTime != 0 && !schema.IsValidField(item.ItemType, "mtime") {
		return fmt.Errorf("field 'mtime' is not valid for itemType %q", item.ItemType)
	}

	for f := range item.ExtraFields {
		if !schema.IsValidField(item.ItemType, f) {
			return fmt.Errorf("field %q is not valid for itemType %q", f, item.ItemType)
		}
	}

	return nil
}
