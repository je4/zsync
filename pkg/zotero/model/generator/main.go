package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

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
}

type LocaleSchema struct {
	ItemTypes    map[string]string `json:"itemTypes"`
	Fields       map[string]string `json:"fields"`
	CreatorTypes map[string]string `json:"creatorTypes"`
}

type Schema struct {
	Version   int                     `json:"version"`
	ItemTypes []ItemTypeSchema        `json:"itemTypes"`
	Locales   map[string]LocaleSchema `json:"locales,omitempty"`
}

func getFieldDoc(field string, enUSFields map[string]string) string {
	label, ok := enUSFields[field]
	if !ok || label == "" {
		switch field {
		case "linkMode":
			return `"Link Mode" (linkMode)`
		case "contentType":
			return `"Content Type" (contentType)`
		case "charset":
			return `"Charset"`
		case "filename":
			return `"Filename"`
		case "md5":
			return `"MD5"`
		case "mtime":
			return `"mtime"`
		default:
			return fmt.Sprintf("%q", field)
		}
	}

	normLabel := strings.ToLower(strings.ReplaceAll(label, " ", ""))
	normField := strings.ToLower(strings.ReplaceAll(field, " ", ""))
	if normLabel != normField {
		return fmt.Sprintf("%q (%s)", label, field)
	}
	return fmt.Sprintf("%q", label)
}

func getItemTypeDoc(itemType string, enUSItemTypes map[string]string) string {
	label, ok := enUSItemTypes[itemType]
	if !ok || label == "" {
		label = toPascalCase(itemType)
	}
	normLabel := strings.ToLower(strings.ReplaceAll(label, " ", ""))
	normType := strings.ToLower(strings.ReplaceAll(itemType, " ", ""))
	if normLabel != normType {
		return fmt.Sprintf("%q (%s)", label, itemType)
	}
	return fmt.Sprintf("%q", label)
}

func toPascalCase(s string) string {
	switch s {
	case "ISBN":
		return "ISBN"
	case "ISSN":
		return "ISSN"
	case "DOI":
		return "DOI"
	case "url":
		return "Url"
	case "md5":
		return "MD5"
	case "mtime":
		return "MTime"
	case "csl":
		return "CSL"
	}
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func itemTypeToStructName(itemType string) string {
	return "Item" + toPascalCase(itemType)
}

var coreFields = map[string]string{
	"title":        "Title",
	"abstractNote": "AbstractNote",
	"date":         "Date",
	"url":          "Url",
	"extra":        "Extra",
	"shortTitle":   "ShortTitle",
	"linkMode":     "LinkMode",
	"note":         "Note",
	"contentType":  "ContentType",
	"charset":      "Charset",
	"filename":     "Filename",
	"md5":          "MD5",
	"mtime":        "MTime",
}

var structFieldNames = map[string]bool{
	// ItemDataBase fields
	"Key":          true,
	"Version":      true,
	"ItemType":     true,
	"Tags":         true,
	"Relations":    true,
	"ParentItem":   true,
	"Collections":  true,
	"DateAdded":    true,
	"DateModified": true,
	"Creators":     true,
	// ItemGeneric core fields
	"Title":        true,
	"AbstractNote": true,
	"Date":         true,
	"Url":          true,
	"Extra":        true,
	"ShortTitle":   true,
	"LinkMode":     true,
	"Note":         true,
	"ContentType":  true,
	"Charset":      true,
	"Filename":     true,
	"MD5":          true,
	"MTime":        true,
	"ExtraFields":  true,
}

var existingMethods = map[string]bool{
	"MarshalJSON":   true,
	"UnmarshalJSON": true,
	"Get":           true,
	"GetString":     true,
	"Set":           true,
	"SetString":     true,
	"Delete":        true,
	"Validate":      true,
	"GetItemType":   true,
	"ToGeneric":     true,
	"FromGeneric":   true,
}

func findSchemaFile(schemaFlag string) (string, error) {
	if schemaFlag != "" {
		if _, err := os.Stat(schemaFlag); err == nil {
			return schemaFlag, nil
		}
	}
	candidates := []string{
		"data/zotero_schema.json",
		"../data/zotero_schema.json",
		"../../data/zotero_schema.json",
		"../../../data/zotero_schema.json",
		"../../../../data/zotero_schema.json",
		"pkg/zotero/model/zotero_schema.json",
		"./zotero_schema.json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find zotero_schema.json in candidate locations")
}

func findOutputDir(outFlag string) (string, error) {
	if outFlag != "" {
		return outFlag, nil
	}
	candidates := []string{
		"pkg/zotero/model",
		".",
		"..",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "itemGeneric.go")); err == nil {
			return c, nil
		}
	}
	return ".", nil
}

func main() {
	schemaFlag := flag.String("schema", "", "Path to zotero_schema.json")
	outFlag := flag.String("out", "", "Output directory for generated Go files")
	flag.Parse()

	schemaPath, err := findSchemaFile(*schemaFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	outDir, err := findOutputDir(*outFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read schema file %s: %v\n", schemaPath, err)
		os.Exit(1)
	}

	var schema Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse schema JSON: %v\n", err)
		os.Exit(1)
	}

	if err := generateItemTypes(outDir, &schema); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate item types: %v\n", err)
		os.Exit(1)
	}

	if err := generateAccessors(outDir, &schema); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate accessors: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated item types and accessors in %s\n", outDir)
}

func generateItemTypes(outDir string, schema *Schema) error {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by go generate; DO NOT EDIT.\n\n")
	buf.WriteString("package model\n\n")
	buf.WriteString("import (\n")
	buf.WriteString("\t\"emperror.dev/errors\"\n")
	buf.WriteString(")\n\n")

	var enUSFields map[string]string
	var enUSItemTypes map[string]string
	if schema.Locales != nil {
		if loc, ok := schema.Locales["en-US"]; ok {
			enUSFields = loc.Fields
			enUSItemTypes = loc.ItemTypes
		}
	}

	// Define ItemData interface
	buf.WriteString("// ItemData represents the common interface implemented by all concrete Zotero item types and ItemGeneric.\n")
	buf.WriteString("type ItemData interface {\n")
	buf.WriteString("\tGetItemType() string\n")
	buf.WriteString("\tToGeneric() *ItemGeneric\n")
	buf.WriteString("\tFromGeneric(gen *ItemGeneric) error\n")
	buf.WriteString("}\n\n")

	// Sort item types for deterministic output
	itemTypes := make([]ItemTypeSchema, len(schema.ItemTypes))
	copy(itemTypes, schema.ItemTypes)
	sort.Slice(itemTypes, func(i, j int) bool {
		return itemTypes[i].ItemType < itemTypes[j].ItemType
	})

	for _, it := range itemTypes {
		structName := itemTypeToStructName(it.ItemType)
		typeDoc := getItemTypeDoc(it.ItemType, enUSItemTypes)
		buf.WriteString(fmt.Sprintf("// %s represents a Zotero %s item.\n", structName, typeDoc))
		buf.WriteString(fmt.Sprintf("type %s struct {\n", structName))
		buf.WriteString("\tItemDataBase\n")

		// Special fields for attachment and note
		fields := make([]FieldSchema, len(it.Fields))
		copy(fields, it.Fields)

		hasField := func(fieldName string) bool {
			for _, f := range fields {
				if f.Field == fieldName {
					return true
				}
			}
			return false
		}

		if it.ItemType == "attachment" {
			attachmentExtra := []string{"linkMode", "note", "contentType", "charset", "filename", "md5"}
			for _, ef := range attachmentExtra {
				if !hasField(ef) {
					fields = append(fields, FieldSchema{Field: ef})
				}
			}
		} else if it.ItemType == "note" {
			if !hasField("note") {
				fields = append(fields, FieldSchema{Field: "note"})
			}
		}

		for _, f := range fields {
			goName := toPascalCase(f.Field)
			fieldDoc := getFieldDoc(f.Field, enUSFields)
			buf.WriteString(fmt.Sprintf("\t// %s represents the %s field.\n", goName, fieldDoc))
			buf.WriteString(fmt.Sprintf("\t%s string `json:\"%s,omitempty\"`\n", goName, f.Field))
		}
		if it.ItemType == "attachment" {
			buf.WriteString("\t// MTime represents the \"mtime\" field.\n")
			buf.WriteString("\tMTime int64 `json:\"mtime,omitzero\"`\n")
		}

		buf.WriteString("}\n\n")

		// GetItemType method
		buf.WriteString(fmt.Sprintf("func (it *%s) GetItemType() string {\n", structName))
		buf.WriteString(fmt.Sprintf("\treturn %q\n", it.ItemType))
		buf.WriteString("}\n\n")

		// ToGeneric method
		buf.WriteString(fmt.Sprintf("func (it *%s) ToGeneric() *ItemGeneric {\n", structName))
		buf.WriteString("\tif it == nil {\n\t\treturn nil\n\t}\n")
		buf.WriteString("\tgen := &ItemGeneric{\n")
		buf.WriteString("\t\tItemDataBase: it.ItemDataBase,\n")
		buf.WriteString(fmt.Sprintf("\t}\n\tgen.ItemType = %q\n", it.ItemType))

		for _, f := range fields {
			goName := toPascalCase(f.Field)
			setterName := "Set" + goName
			buf.WriteString(fmt.Sprintf("\tgen.%s(it.%s)\n", setterName, goName))
		}
		if it.ItemType == "attachment" {
			buf.WriteString("\tgen.MTime = it.MTime\n")
		}
		buf.WriteString("\treturn gen\n}\n\n")

		// FromGeneric method
		buf.WriteString(fmt.Sprintf("func (it *%s) FromGeneric(gen *ItemGeneric) error {\n", structName))
		buf.WriteString("\tif gen == nil {\n\t\treturn errors.New(\"cannot populate from nil ItemGeneric\")\n\t}\n")
		buf.WriteString("\tit.ItemDataBase = gen.ItemDataBase\n")
		buf.WriteString(fmt.Sprintf("\tit.ItemType = %q\n", it.ItemType))

		for _, f := range fields {
			goName := toPascalCase(f.Field)
			if _, isCore := coreFields[f.Field]; isCore {
				buf.WriteString(fmt.Sprintf("\tit.%s = gen.%s\n", goName, goName))
			} else {
				buf.WriteString(fmt.Sprintf("\tit.%s = gen.%s()\n", goName, goName))
			}
		}
		if it.ItemType == "attachment" {
			buf.WriteString("\tit.MTime = gen.MTime\n")
		}
		buf.WriteString("\treturn nil\n}\n\n")
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt error on item types: %w\nRaw:\n%s", err, buf.String())
	}

	outPath := filepath.Join(outDir, "item_types_gen.go")
	return os.WriteFile(outPath, formatted, 0644)
}

func generateAccessors(outDir string, schema *Schema) error {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by go generate; DO NOT EDIT.\n\n")
	buf.WriteString("package model\n\n")

	var enUSFields map[string]string
	if schema.Locales != nil {
		if loc, ok := schema.Locales["en-US"]; ok {
			enUSFields = loc.Fields
		}
	}

	// Collect all unique field names
	uniqueFields := make(map[string]struct{})
	for _, it := range schema.ItemTypes {
		for _, f := range it.Fields {
			uniqueFields[f.Field] = struct{}{}
		}
	}
	// Add note and attachment specific fields
	attachmentExtra := []string{"linkMode", "note", "contentType", "charset", "filename", "md5"}
	for _, f := range attachmentExtra {
		uniqueFields[f] = struct{}{}
	}

	fieldList := make([]string, 0, len(uniqueFields))
	for f := range uniqueFields {
		fieldList = append(fieldList, f)
	}
	sort.Strings(fieldList)

	for _, field := range fieldList {
		goName := toPascalCase(field)
		fieldDoc := getFieldDoc(field, enUSFields)

		// Check if getter conflicts with struct field or method
		if !structFieldNames[goName] && !existingMethods[goName] {
			buf.WriteString(fmt.Sprintf("// %s returns the %s field value.\n", goName, fieldDoc))
			buf.WriteString(fmt.Sprintf("func (item *ItemGeneric) %s() string {\n", goName))
			buf.WriteString(fmt.Sprintf("\treturn item.GetString(%q)\n", field))
			buf.WriteString("}\n\n")
		}

		// Setter
		setterName := "Set" + goName
		if !existingMethods[setterName] {
			buf.WriteString(fmt.Sprintf("// %s sets the %s field value.\n", setterName, fieldDoc))
			buf.WriteString(fmt.Sprintf("func (item *ItemGeneric) %s(val string) {\n", setterName))
			if _, isCore := coreFields[field]; isCore && field != "mtime" {
				buf.WriteString(fmt.Sprintf("\titem.%s = val\n", goName))
			} else {
				buf.WriteString(fmt.Sprintf("\titem.Set(%q, val)\n", field))
			}
			buf.WriteString("}\n\n")
		}
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt error on accessors: %w\nRaw:\n%s", err, buf.String())
	}

	outPath := filepath.Join(outDir, "item_accessors_gen.go")
	return os.WriteFile(outPath, formatted, 0644)
}
