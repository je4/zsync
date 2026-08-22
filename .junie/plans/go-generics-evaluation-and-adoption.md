---
sessionId: session-260822-131906-nk82
---

# Requirements

### Overview & Goals
Mit modernen Go-Versionen (einschließlich der in `go.work` und `go.mod` konfigurierten Toolchain) bieten Generics (Type Parameters für Funktionen und Typen) sowie die erweiterten Standard-Pakete `slices`, `maps` und `cmp` leistungsfähige Werkzeuge zur Erhöhung der Typsicherheit und Beseitigung von Code-Duplikation.

Das Ziel dieser Analyse und des Umsetzungsplans ist es:
1. Zu bewerten, **wo** der Einsatz von Generics in `zsync` einen echten Mehrwert stiftet (hoher ROI bezüglich Typsicherheit, Lesbarkeit und Wartbarkeit).
2. Zu identifizieren, **wo Generics vermieden werden sollten** (Anti-Patterns wie unnötige Abstraktionen, Verdrängung von Interfaces oder Über-Komplizierung der Datenmodelle).
3. Eine strukturierte Roadmap für die gezielte Einführung von generischen Helfern und Refactorings in `pkg/zotero/client`, `pkg/zotero/sync` und `cmd/rest` bereitzustellen.

---

### Scope

#### In Scope (Sinnvolle Einsatzbereiche)
- **Generischer REST-API Paginator**: Zentralisierung der duplizierten Paginierungs- und Backoff-Schleifen in `pkg/zotero/client/` (`item.go`, `collection.go`, `tag.go`, `user.go`).
- **Standard-Bibliotheks-Generics (`slices`, `maps`)**: Konsequente Ablösung manueller Iterations- und Kopiermuster durch `slices.Contains`, `slices.Chunk`, `slices.Compact`, `maps.Keys`, `maps.Copy`.
- **Generische Batching- und Chunk-Helfer**: Sicheres Aufteilen von ID-/Key-Listen in 50er-Batches für Zotero API Calls in `syncer.go`.
- **Migration auf `otter/v2` Cache**: Vollständige Ablösung von `gcache` durch die native generische Cache-Bibliothek `otter` v2 (`github.com/maypok86/otter/v2`) in `cmd/rest/handler.go` (`*otter.Cache[int64, *model.Group]`) zur Beseitigung ungesicherter Type-Assertions (`tmp.(*model.Group)`).
- **Generisches `Set[T comparable]`**: Effiziente Mengenoperationen (Diff, Intersection, Deduplizierung) für Item- und Collection-Keys.

#### Out of Scope (Wo Generics keinen Sinn machen / Anti-Patterns)
- **Kein Ersatz von Interfaces durch Generics**: `filesystem.FileSystem`, `zLogger.ZLogger` und `pgx.Row` bleiben polymorphe Interfaces für Verhalten.
- **Keine generische Über-Abstraktion des Zotero-Domänenmodells**: `ItemGeneric`, `ItemDataBase`, `ItemNote`, `ItemAttachment` verbleiben als konkrete Structs mit `encoding/json/v2`-Tags.
- **Kein generisches ORM / Storage-Mapping**: pgx SQL-Row-Scans bleiben typspezifisch und transparent.

---

### User Stories
- **Als Entwickler** möchte ich wiederkehrende API-Paginierungslogik nur an einer zentralen Stelle pflegen, damit Fehlerbehandlung, Retry- und Backoff-Regeln für alle Zotero-Entitäten einheitlich funktionieren.
- **Als Entwickler** möchte ich typsichere Collections und Hilfsfunktionen nutzen, um Laufzeit-Panics durch Type-Casts (`interface{}` / `any`) bereits zur Compile-Zeit auszuschließen.
- **Als Maintainer** möchte ich idiomatischen, kompakten Go-Code, der die Standardbibliothek optimal nutzt, ohne unnötige Abstraktionsschichten einzuführen.

---

### Functional Requirements
- **FR-1**: Bereitstellung eines generischen Paginators `fetchPaginated[T any]`, der Header (`Total-Results`, `Last-Modified-Version`), Retry-Mechanismen und JSON-Deserialisierung kapselt.
- **FR-2**: Bereitstellung von Chunking- und Slice-Transformationsfunktionen für Batch-Operationen.
- **FR-3**: Vollständige Migration des In-Memory Caches in `cmd/rest/handler.go` von `github.com/bluele/gcache` auf `github.com/maypok86/otter/v2` (`*otter.Cache[int64, *model.Group]`).
- **FR-4**: Volle Rückwärtskompatibilität aller bestehenden Schnittstellen und fehlerfreie Ausführung der Integrations- und Mock-Tests.

---

### Non-Functional Requirements & Decision Matrix

| Kriterium | Ohne Generics (Status Quo) | Mit gezielten Generics (Empfehlung) | Übermäßiger Generics-Einsatz |
| :--- | :--- | :--- | :--- |
| **Typsicherheit** | Mittel (Runtime-Casts bei Caches & JSON) | **Hoch (Compile-Time Checks)** | Hoch |
| **Code-Duplikation** | Hoch (~300 Zeilen duplizierte Paginierung) | **Sehr gering (Single Source of Truth)** | Gering |
| **Lesbarkeit** | Gut, aber repetitiv | **Sehr gut und idiomatisch** | Schlecht (Typ-Signatur-Bloat) |
| **Performance** | Gut | **Identisch / Monomorphisiert (Zero Runtime Overhead)** | Ggf. Compile-Zeit Verlangsamung |

# Technical Design

### Current Implementation
Die Untersuchung des Repositories zeigt folgende Ausgangslage:
1. **Paginierung in `pkg/zotero/client`**:
   - `GetItemsVersion` (`item.go`), `GetCollectionVersions` (`collection.go`), `GetTagsVersion` (`tag.go`) und `GetUserGroupVersions` (`user.go`) implementieren fast identische `for { ... limit/start/since ... }` Schleifen mit identischem Header-Parsing und Backoff-Handling.
2. **Chunking / Batching in `pkg/zotero/sync/syncer.go`**:
   - Manuelle Index-Berechnung für 50er-Batches (`numCollections/50 + 1`, `collectionUpdate[start:end]`) wird an mehreren Stellen wiederholt.
3. **Caching in `cmd/rest/handler.go`**:
   - `gcache.Cache` speichert `interface{}` und erfordert explizite Runtime-Assertions wie `group, ok = tmp.(*model.Group)`.

---

### Key Decisions

#### Decision 1: Gezielter Einsatz für Infrastruktur & Algorithmen, nicht für Domänenmodelle
- **Entscheidung**: Generics werden strikt für Algorithmen (Paginierung, Chunking, Set-Operationen) und generische Bibliotheken (`otter/v2`) eingesetzt. Domänenmodelle (`model.Item`, `model.Collection`) bleiben unverändert.
- **Rationale**: Go-Generics glänzen bei datenstruktur-unabhängigen Algorithmen. Zotero-Domänenobjekte profitieren mehr von klarer Deklaration und expliziter JSON-Serialisierung.

#### Decision 2: Umstellung auf `otter/v2` statt Custom-Wrapper um `gcache`
- **Entscheidung**: `gcache` (`github.com/bluele/gcache`) wird komplett durch `otter` v2 (`github.com/maypok86/otter/v2`) ersetzt, anstatt einen eigenen generischen Wrapper um die veraltete `gcache`-Bibliothek zu bauen.
- **Rationale**: `otter` v2 ist eine moderne, extrem performante (S3-FIFO, lock-contention-frei) und nativ mit Go-Generics entwickelte Cache-Bibliothek (`*otter.Cache[K, V]`). Ein nativer Umstieg eliminiert unnötigen Boilerplate-Code, entfernt veraltete `interface{}`-Abhängigkeiten und bietet erstklassige Concurrency- und TTL-Unterstützung.

#### Decision 3: Priorisierung der Standardbibliothek (`slices`, `maps`, `cmp`) vor eigenen Hilfspaketen
- **Entscheidung**: Wann immer möglich, werden die generischen Funktionen der Go-Standardbibliothek genutzt. Nur fehlende Spezialisierungen (z. B. `Chunk` für ältere Pipelines, `Set[T]`, `ExtractKeys`) wandern in ein internes Hilfspaket.
- **Rationale**: Minimaler Wartungsaufwand und maximale Standardkonformität.

---

### Proposed Changes & Code Examples

#### 1. Generischer API Paginator (`pkg/zotero/client/paginator.go`)
```go
package client

import (
	"encoding/json/v2"
	"strconv"
	"emperror.dev/errors"
	"gopkg.in/resty.v1"
)

type PageResult[T any] struct {
	Items               []T
	LastModifiedVersion int64
}

func fetchPaginated[T any](c *Client, endpoint string, limit int64, setupParams func(*resty.Request)) (*PageResult[T], error) {
	var results []T
	var lastModifiedVersion int64
	var start int64 = 0

	for {
		req := c.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))

		if setupParams != nil {
			setupParams(req)
		}

		var resp *resty.Response
		var err error
		for {
			resp, err = req.Get(endpoint)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot fetch from %s", endpoint)
			}
			if !c.CheckRetry(resp.Header()) {
				break
			}
		}

		var pageItems []T
		if err := json.Unmarshal(resp.Body(), &pageItems); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal response from %s", endpoint)
		}

		results = append(results, pageItems...)
		c.CheckBackoff(resp.Header())

		// Parse headers
		limv := resp.RawResponse.Header.Get("Last-Modified-Version")
		if h, err := strconv.ParseInt(limv, 10, 64); err == nil && h > lastModifiedVersion {
			lastModifiedVersion = h
		}

		totalResultStr := resp.RawResponse.Header.Get("Total-Results")
		if totalResult, err := strconv.ParseInt(totalResultStr, 10, 64); err == nil {
			if int64(len(results)) >= totalResult || totalResult <= start+limit {
				break
			}
		} else if int64(len(pageItems)) < limit {
			break
		}

		start += limit
	}

	return &PageResult[T]{Items: results, LastModifiedVersion: lastModifiedVersion}, nil
}
```

#### 2. Generischer Chunking & Key-Extractor (`pkg/utils/generics.go`)
```go
package utils

// Chunk teilt ein Slice beliebigen Typs in Chunks der angegebenen Größe auf.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 || len(items) == 0 {
		return nil
	}
	var chunks [][]T
	for size < len(items) {
		items, chunks = items[size:], append(chunks, items[0:size:size])
	}
	return append(chunks, items)
}

// ExtractKeys extrahiert ein Feld beliebigen Typs aus einer Slice von Objekten.
func ExtractKeys[T any, K comparable](items []T, selector func(T) K) []K {
	keys := make([]K, len(items))
	for i, item := range items {
		keys[i] = selector(item)
	}
	return keys
}
```

#### 3. Migration auf `otter/v2` Cache (`cmd/rest/handler.go`)
```go
package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"emperror.dev/errors"
	"github.com/maypok86/otter/v2"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
)

type Handlers struct {
	groups  *otter.Cache[int64, *model.Group]
	cfg     *Config
	logger  *logging.Logger
	storage *storage.Storage
	client  *client.Client
	fs      filesystem.FileSystem
}

func NewHandler(storage *storage.Storage, client *client.Client, fs filesystem.FileSystem, cfg *Config, logger *logging.Logger) *Handlers {
	exp, err := time.ParseDuration(cfg.GroupCacheExpiration)
	if err != nil {
		log.Fatalf("error parsing expiration: %v", err)
	}

	cache, err := otter.New(&otter.Options[int64, *model.Group]{
		MaximumSize:      500,
		ExpiryCalculator: otter.ExpiryWriting[int64, *model.Group](exp),
	})
	if err != nil {
		log.Fatalf("error initializing otter cache: %v", err)
	}

	handlers := &Handlers{
		storage: storage,
		client:  client,
		fs:      fs,
		cfg:     cfg,
		logger:  logger,
		groups:  cache,
	}
	return handlers
}

func (handlers *Handlers) getGroup(groupId int64) (*model.Group, error) {
	group, ok := handlers.groups.GetIfPresent(groupId)
	if !ok {
		var err error
		group, err = handlers.storage.LoadGroup(groupId)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot load group %v", groupId)
		}
		handlers.groups.Set(groupId, group)
	}
	return group, nil
}
```

---

### Architecture Diagram

```mermaid
graph TD
    A[cmd/sync, cmd/rest] -->|Verwendet| B[pkg/zotero/sync]
    B -->|Generisches Chunking / Slices| C[Generic Utils / stdlib slices]
    B -->|Batch Calls| D[pkg/zotero/client]
    D -->|fetchPaginated[T]| E[Generic Paginator Engine]
    E -->|Deserialisiert| F[model.Item / model.Collection / model.Tag]
    A -->|*otter.Cache[int64, *model.Group]| G[otter v2 In-Memory Cache]
```

---

### Risks & Anti-Patterns to Avoid

| Risiko / Anti-Pattern | Warum vermeiden? | Bessere Alternative |
| :--- | :--- | :--- |
| **Generische Interfaces für Logging / FS** | Zerstört Entkopplung und erfordert Typ-Propagation in allen Konstruktoren. | Klassische Go-Interfaces (`zLogger.ZLogger`, `filesystem.FileSystem`). |
| **`Item[T Data]` statt `ItemGeneric`** | Zotero hat polymorphe Item-Typen mit dynamischer JSON-Struktur; Generics erzwingen statische Typen. | Konkrete Structs mit Embedding (`ItemDataBase`). |
| **Unnötige generische Wrapper für 1-Zeiler** | Erhöht Cognitive Load ohne Mehrwert. | Direkte Verwendung von `slices.Contains` oder Standard-Go-Loops. |

# Testing

### Validation Approach
Die Überprüfung der generischen Refactorings erfolgt über automatische Unit- und Integrationstests, um sicherzustellen, dass:
1. Keine semantischen Änderungen im API-Client-Verhalten oder Header-Parsing auftreten.
2. Typsicherheit und Fehlerbehandlung bei Caches und Paginatoren erhalten bleiben.
3. Keine Memory-Leaks oder Performance-Regressionen entstehen.

---

### Key Scenarios
- **Paginator-Vollständigkeit**: Test mit Zotero-Mock-Server für >500 Items und >100 Tags, um zu validieren, dass alle Seiten korrekt aggregiert und `Last-Modified-Version` korrekt ermittelt werden.
- **Batching & Chunking**: Validierung, dass Chunks exakt bei 50 Elementen getrennt werden und leere/ungerade Rest-Slices fehlerfrei verarbeitet werden.
- **Cache Hit / Miss & TTL**: Prüfung des `*otter.Cache[int64, *model.Group]` auf Cache-Hits (`GetIfPresent`), Verfall nach TTL (`ExpiryWriting` mit `GroupCacheExpiration`) und fehlerfreie typsichere Rückgabe ohne Type-Cast-Fehler.

---

### Edge Cases
- Leere Antwortlisten von Zotero API (`Total-Results: 0` oder leeres JSON-Array `[]`).
- Fehlende Header (`Total-Results` oder `Last-Modified-Version` nicht gesetzt).
- Ungültige Chunk-Größen (`size <= 0`) bei `Chunk[T]`.

# Delivery Steps

### ✓ Step 1: Generisches Utility-Paket und Standardbibliothek-Modernisierung einfuehren
Ein zentrales Utility-Paket für generische Hilfsfunktionen steht bereit und bestehende Ad-hoc-Logiken nutzen Standard-Generics (`slices`, `maps`, `cmp`).

- Anlegen von generischen Slice- und Map-Hilfsfunktionen (z. B. `Chunk[T]`, `Filter[T]`, `Map[T, R]`, `ExtractKeys[T, K comparable]`).
- Ersetzen manueller Slice- und Map-Operationen in `pkg/zotero/sync/syncer.go` und `cmd/sync/main.go` durch Standardbibliotheksfunktionen (`slices.Contains`, `slices.Chunk`, `maps.Keys`, `maps.Copy`).
- Bereitstellung einer leichtgewichtigen, generischen Datenstruktur `Set[T comparable]` für Deduplizierung und Mengenoperationen bei Zotero-Keys.
- Unit-Tests für alle neuen generischen Hilfsfunktionen zur Sicherstellung der Korrektheit und Performance.

### ✓ Step 2: Generischen API-Paginator im Zotero-Client implementieren
Redundante Paginierungs- und API-Batch-Routinen im Zotero-Client sind durch einen einheitlichen generischen Paginator ersetzt.

- Entwurf und Implementierung von `fetchPaginated[T any](c *Client, endpoint string, params map[string]string) ([]T, int64, error)` in `pkg/zotero/client/`.
- Refactoring von `GetCollectionVersions`, `GetItemsVersion`, `GetTagsVersion` und `GetUserGroupVersions` zur Nutzung des generischen Paginators.
- Eliminierung duplizierter Header-Parsing- (`Total-Results`, `Last-Modified-Version`), Backoff- und Retry-Logiken.
- Anpassung und Verifikation der bestehenden Client-Tests (`client_test.go`, `cloud_api_test.go`, `local_api_test.go`).

### ✓ Step 3: Syncer-Batching auf Generics und REST-Caching auf otter v2 umstellen
Syncer-Batching verwendet generische Helper und der REST-Dienst nutzt `otter` v2 als nativen generischen Cache ohne `gcache` und ohne Laufzeit-Type-Assertions.

- Refactoring der Batching-Schleifen in `pkg/zotero/sync/syncer.go` (`SyncCollections`, `DownloadItems`) auf generisches Chunking (`slices.Chunk`).
- Ablösung von `github.com/bluele/gcache` durch `github.com/maypok86/otter/v2` (`*otter.Cache[int64, *model.Group]`) in `cmd/rest/handler.go`.
- Bereinigung von `go.mod` (Entfernen von `gcache`, Hinzufügen von `otter/v2`).
- Modernisierung von `respondWithJSON` und Request-Parsing-Helfern in `cmd/rest/handler*.go`.
- Validierung der Synchronisations- und API-Endpunkte über Integrationstests (`sync_test.go`, `storage_test.go`).