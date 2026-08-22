package model

//go:generate go run ./generator

import (
	_ "embed"
	"encoding/json/v2"
	"fmt"
	"sync"
)

//go:embed zotero_schema.json
var schemaJSON []byte

type FieldSchema struct {
	Field     string `json:"field"`
	BaseField string `json:"baseField,omitempty"`
}

type CreatorTypeSchema struct {
	CreatorType string `json:"creatorType"`
	Primary     bool   `json:"primary,omitempty"`
}

type ItemTypeSchema struct {
	ItemType     string              `json:"itemType"`
	Fields       []FieldSchema       `json:"fields"`
	CreatorTypes []CreatorTypeSchema `json:"creatorTypes"`

	fieldSet       map[string]struct{}
	creatorTypeSet map[string]struct{}
}

type SchemaMeta struct {
	Fields       map[string]any `json:"fields,omitempty"`
	ItemTypes    map[string]any `json:"itemTypes,omitempty"`
	CreatorTypes map[string]any `json:"creatorTypes,omitempty"`
	CSL          map[string]any `json:"csl,omitempty"`
	Locales      map[string]any `json:"locales,omitempty"`
}

type Schema struct {
	Version   int              `json:"version"`
	ItemTypes []ItemTypeSchema `json:"itemTypes"`
	Meta      SchemaMeta       `json:"meta,omitempty"`

	itemTypeMap map[string]*ItemTypeSchema
}

var (
	defaultSchema     *Schema
	defaultSchemaOnce sync.Once
	defaultSchemaErr  error
)

// GetSchema returns the parsed singleton Zotero schema.
func GetSchema() *Schema {
	defaultSchemaOnce.Do(func() {
		defaultSchema, defaultSchemaErr = parseSchema(schemaJSON)
	})
	if defaultSchemaErr != nil {
		panic(fmt.Sprintf("failed to parse embedded zotero schema: %v", defaultSchemaErr))
	}
	return defaultSchema
}

func parseSchema(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.itemTypeMap = make(map[string]*ItemTypeSchema, len(s.ItemTypes))
	for i := range s.ItemTypes {
		it := &s.ItemTypes[i]
		it.fieldSet = make(map[string]struct{}, len(it.Fields))
		for _, f := range it.Fields {
			it.fieldSet[f.Field] = struct{}{}
		}
		it.creatorTypeSet = make(map[string]struct{}, len(it.CreatorTypes))
		for _, ct := range it.CreatorTypes {
			it.creatorTypeSet[ct.CreatorType] = struct{}{}
		}
		s.itemTypeMap[it.ItemType] = it
	}
	return &s, nil
}

// IsValidItemType reports whether the given itemType is recognized by the schema.
func IsValidItemType(itemType string) bool {
	return GetSchema().IsValidItemType(itemType)
}

func (s *Schema) IsValidItemType(itemType string) bool {
	_, ok := s.itemTypeMap[itemType]
	return ok
}

// GetItemTypeSchema returns the schema for the given itemType if present.
func GetItemTypeSchema(itemType string) (*ItemTypeSchema, bool) {
	return GetSchema().GetItemTypeSchema(itemType)
}

func (s *Schema) GetItemTypeSchema(itemType string) (*ItemTypeSchema, bool) {
	it, ok := s.itemTypeMap[itemType]
	return it, ok
}

// IsValidField reports whether field is a valid schema field for the given itemType.
func IsValidField(itemType, field string) bool {
	return GetSchema().IsValidField(itemType, field)
}

func (s *Schema) IsValidField(itemType, field string) bool {
	it, ok := s.itemTypeMap[itemType]
	if !ok {
		return false
	}
	if _, ok := it.fieldSet[field]; ok {
		return true
	}
	if itemType == "note" && field == "note" {
		return true
	}
	if itemType == "attachment" {
		switch field {
		case "linkMode", "note", "contentType", "charset", "filename", "md5", "mtime":
			return true
		}
	}
	return false
}

// GetValidFields returns the list of valid field names for the given itemType.
func GetValidFields(itemType string) []string {
	return GetSchema().GetValidFields(itemType)
}

func (s *Schema) GetValidFields(itemType string) []string {
	it, ok := s.itemTypeMap[itemType]
	if !ok {
		return nil
	}
	fields := make([]string, 0, len(it.Fields)+7)
	for _, f := range it.Fields {
		fields = append(fields, f.Field)
	}
	if itemType == "note" {
		fields = append(fields, "note")
	}
	if itemType == "attachment" {
		fields = append(fields, "linkMode", "note", "contentType", "charset", "filename", "md5", "mtime")
	}
	return fields
}

// IsValidCreatorType reports whether creatorType is valid for the given itemType.
func IsValidCreatorType(itemType, creatorType string) bool {
	return GetSchema().IsValidCreatorType(itemType, creatorType)
}

func (s *Schema) IsValidCreatorType(itemType, creatorType string) bool {
	it, ok := s.itemTypeMap[itemType]
	if !ok {
		return false
	}
	_, ok = it.creatorTypeSet[creatorType]
	return ok
}

// GetValidCreatorTypes returns the list of valid creator types for the given itemType.
func GetValidCreatorTypes(itemType string) []string {
	return GetSchema().GetValidCreatorTypes(itemType)
}

func (s *Schema) GetValidCreatorTypes(itemType string) []string {
	it, ok := s.itemTypeMap[itemType]
	if !ok {
		return nil
	}
	types := make([]string, 0, len(it.CreatorTypes))
	for _, ct := range it.CreatorTypes {
		types = append(types, ct.CreatorType)
	}
	return types
}

// ValidateItem validates that the item's itemType, creators, and fields conform to the schema.
func ValidateItem(item *ItemGeneric) error {
	if item == nil {
		return fmt.Errorf("item is nil")
	}
	return item.Validate()
}
