---
sessionId: session-260822-165443-ksal
---

# Requirements

### Overview & Goals
Hinzufügen von englischer Dokumentation (Go-Doc-Kommentare) für alle generierten Item-Typ-Felder sowie die generierten Getter und Setter von `ItemGeneric`.
Als Informationsquelle für die Beschriftungen und Bezeichnungen dient das `en-US`-Locale aus `zotero_schema.json`.

Bisher erzeugte der Generator (`pkg/zotero/model/generator/main.go`):
- Für Struct-Felder in `item_types_gen.go` überhaupt keine Doc-Kommentare.
- Für Getter und Setter in `item_accessors_gen.go` lediglich rudimentäre Kommentare mit dem rohen Schema-Feldnamen (z. B. `// ArtworkMedium returns the artworkMedium field value.`).

Mit dieser Änderung werden alle Struct-Felder, Item-Typ-Definitionen sowie Getter und Setter mit präzisen, standardkonformen Go-Doc-Kommentaren basierend auf den englischen Bezeichnungen (z. B. `"Medium"` für `artworkMedium`, `"Abstract"` für `abstractNote`) dokumentiert.

### Scope
- **In Scope**:
  - Erweiterung des Schema-Parsings im Generator (`pkg/zotero/model/generator/main.go`), um `locales["en-US"]` (`fields`, `itemTypes`) einzulesen.
  - Generierung von Go-Doc-Kommentaren für Struct-Felder in `item_types_gen.go` unter Verwendung des englischen Feldlabels aus dem `en-US`-Locale (mit Fallback auf den Feldnamen).
  - Verbesserung der Go-Doc-Kommentare für `Item<Type>`-Structs (unter Verwendung des englischen Item-Type-Labels, z. B. `"Artwork"` für `artwork`).
  - Verbesserung der Go-Doc-Kommentare für Getter und Setter in `item_accessors_gen.go` (z. B. `// ArtworkMedium returns the "Medium" (artworkMedium) field value.`).
  - Neu-Generierung von `pkg/zotero/model/item_types_gen.go` und `pkg/zotero/model/item_accessors_gen.go`.
  - Ausführen und Validieren aller Tests in `pkg/zotero/model`.
- **Out of Scope**:
  - Modifikationen an `zotero_schema.json`.
  - Änderungen an der Laufzeitlogik von `ItemGeneric` oder konkreten Item-Methoden.

### User Stories
- **Als Entwickler** möchte ich beim Arbeiten mit Zotero-Item-Typen, Gettern und Settern in der IDE englische Hover-Dokumentation und Beschreibungen sehen, die den tatsächlichen Zotero-Feldbezeichnungen (z. B. „Publication Title“, „Date Added“, „Medium“) entsprechen, um Felder schneller und fehlerfreier zuzuordnen.

### Functional Requirements
- Der Generator liest das `en-US`-Locale (`fields` und `itemTypes`) aus `zotero_schema.json`.
- Jedes Struct-Feld in `item_types_gen.go` erhält einen Go-Doc-Kommentar der Form:
  - Wenn Label vorhanden und abweichend: `// <FieldName> represents the "<Label>" (<rawField>) field.`
  - Wenn Label identisch oder kein Schema-Key: `// <FieldName> represents the "<Label>" field.`
- Jeder Getter in `item_accessors_gen.go` erhält einen Go-Doc-Kommentar:
  - `// <FieldName> returns the "<Label>" (<rawField>) field value.` (bzw. `// <FieldName> returns the "<Label>" field value.`)
- Jeder Setter in `item_accessors_gen.go` erhält einen Go-Doc-Kommentar:
  - `// Set<FieldName> sets the "<Label>" (<rawField>) field value.` (bzw. `// Set<FieldName> sets the "<Label>" field value.`)
- Die generierten Dateien werden mit `go/format` formatiert und kompilieren fehlerfrei.

# Technical Design

### Current Implementation
- `pkg/zotero/model/generator/main.go`:
  - `Schema`-Struct ignoriert `locales`:
    ```go
    type Schema struct {
        Version   int              `json:"version"`
        ItemTypes []ItemTypeSchema `json:"itemTypes"`
    }
    ```
  - `generateItemTypes` generiert Structs und Felder ohne Kommentare:
    ```go
    for _, f := range fields {
        goName := toPascalCase(f.Field)
        buf.WriteString(fmt.Sprintf("\t%s string `json:\"%s,omitempty\"`\n", goName, f.Field))
    }
    ```
  - `generateAccessors` generiert rudimentäre Kommentare:
    ```go
    buf.WriteString(fmt.Sprintf("// %s returns the %s field value.\n", goName, field))
    ```

### Key Decisions
1. **Locale-Datenstruktur im Generator**:
   ```go
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
   ```
2. **Format der Go-Doc-Kommentare**:
   - Für ein Feld wie `artworkMedium` mit `en-US`-Label `"Medium"`:
     - Struct-Feld: `// ArtworkMedium represents the "Medium" (artworkMedium) field.`
     - Getter: `// ArtworkMedium returns the "Medium" (artworkMedium) field value.`
     - Setter: `// SetArtworkMedium sets the "Medium" (artworkMedium) field value.`
   - Für ein Feld wie `title` mit `en-US`-Label `"Title"`:
     - Struct-Feld: `// Title represents the "Title" field.`
     - Getter: `// Title returns the "Title" field value.`
     - Setter: `// SetTitle sets the "Title" field value.`
   - Für Spezialfelder wie `mtime` (falls nicht in `en-US` vorhanden):
     - Fallback: `// MTime represents the modification time ("mtime") field.`

### Proposed Changes

#### 1. `pkg/zotero/model/generator/main.go`
- Ergänzung von `LocaleSchema` und `Schema.Locales`.
- Hilfsfunktion `getFieldDoc(field string, enUSFields map[string]string) string`:
  - Liefert formatierten Beschreibungsstring wie `"\"Medium\" (artworkMedium)"` oder `"\"Title\""`.
- Anpassung von `generateItemTypes`:
  - Erzeugen von Kommentaren vor jedem Struct-Feld.
  - Erzeugen von Typ-Kommentaren wie `// ItemArtwork represents a Zotero "Artwork" item (artwork).`.
- Anpassung von `generateAccessors`:
  - Verwenden von `getFieldDoc` für Getter- und Setter-Kommentare.

#### 2. `pkg/zotero/model/item_types_gen.go` & `item_accessors_gen.go`
- Automatische Neu-Generierung durch `go generate ./pkg/zotero/model`.

### File Structure
- `pkg/zotero/model/generator/main.go` — Generator-Logik mit `en-US`-Locale-Support
- `pkg/zotero/model/item_types_gen.go` — Generierte Typen mit Feldkommentaren
- `pkg/zotero/model/item_accessors_gen.go` — Generierte Accessors mit englischen Beschreibungen

# Testing

### Validation Approach
- Ausführen von `go generate ./pkg/zotero/model` und Überprüfung der generierten Dateien auf saubere Formatierung und fehlerfreie Kommentare.
- Ausführen von `go test -v ./pkg/zotero/model/...` zur Sicherstellung, dass alle Tests weiterhin bestehen.

### Key Scenarios
1. **Generierte Feldkommentare in `item_types_gen.go`**:
   - Überprüfen, dass Felder wie `ArtworkMedium`, `PublicationTitle`, `AbstractNote` etc. die passenden englischen Bezeichnungen aus `en-US` im Doc-Kommentar tragen.
2. **Generierte Accessor-Kommentare in `item_accessors_gen.go`**:
   - Überprüfen der Getter/Setter-Kommentare für Core- und Nicht-Core-Felder.
3. **Kompilierung & Test-Suite**:
   - Sicherstellen, dass keine Syntaxfehler oder Formatierungsfehler durch die Kommentare entstehen.

### Test Changes
- Keine neuen Testfälle zwingend erforderlich; bestehende Tests validieren die Funktionalität und syntaktische Korrektheit der generierten Typen.

# Delivery Steps

### ✓ Step 1: Add en-US locale parsing and doc comment formatting to generator
Update `pkg/zotero/model/generator/main.go` to parse `en-US` locale information and generate English Go-doc comments:
- Define `LocaleSchema` and include `Locales map[string]LocaleSchema` in `Schema`.
- Add helper function `getFieldDoc` to format field descriptions using `en-US` labels.
- Update `generateItemTypes` to emit Go-doc comments on struct definitions and struct fields.
- Update `generateAccessors` to emit enhanced Go-doc comments on getters and setters.

### ✓ Step 2: Regenerate item types and accessors and verify tests
Execute code generator and validate model tests:
- Run `go generate ./pkg/zotero/model` to update `item_types_gen.go` and `item_accessors_gen.go`.
- Verify formatting and clean Go-doc comments.
- Run `go test -v ./pkg/zotero/model/...` to confirm that all tests pass without errors.