---
sessionId: session-260822-124901-1m4m
---

# Requirements

### Overview & Goals
The project was migrated to Go's experimental `encoding/json/v2`. Several compilation errors and unit test failures resulted from API differences between `encoding/json` (v1) and `encoding/json/v2` (v2), as well as changes in struct tag semantics. The goal is to resolve all build errors and failing tests so that the entire repository compiles and passes all tests under `encoding/json/v2`.

### Scope
- **In Scope**:
  - Replace unavailable v1 APIs (`json.MarshalIndent`, `json.NewEncoder`, `json.NewDecoder`) with their v2 equivalents (`json.Marshal(..., jsontext.WithIndent("  "))`, `json.MarshalWrite`, `json.UnmarshalRead`).
  - Adapt struct tags for zero values (`omitzero`) where v2's `omitempty` no longer omits numeric `0`.
  - Fix compiler format string warning in `pkg/filesystem/S3Fs.go`.
  - Ensure all packages build cleanly with `go build ./...` and all tests pass with `go test ./...`.
- **Out of Scope**:
  - Reverting from `encoding/json/v2` back to `encoding/json` v1.
  - Modifying business logic unrelated to JSON serialization or compilation errors.

### Functional Requirements
- `pkg/zotero/storage`: Group serialization with indentation functions properly using `jsontext.WithIndent("  ")`.
- `cmd/rest`: HTTP handlers unmarshal JSON requests using `json.UnmarshalRead(r.Body, &dst)`.
- `pkg/zotero/client` and `pkg/zotero/sync` tests: Mock HTTP servers write and read JSON using `json.MarshalWrite` and `json.UnmarshalRead`.
- `pkg/zotero/model`: `Version: 0` in `CollectionData` and `ItemDataBase` is omitted during JSON serialization as expected by the model test suite.

# Technical Design

### Current State & Root Cause Analysis

1. **`json.MarshalIndent` removed in v2**:
   - `encoding/json/v2` dropped `MarshalIndent`.
   - Solution: Use `json.Marshal(v, jsontext.WithIndent("  "))` with `import "encoding/json/jsontext"`.
   - Affected files:
     - `pkg/zotero/storage/group.go` (line 154)
     - `pkg/zotero/sync/syncer.go` (lines 538, 571, 614)

2. **`json.NewEncoder` and `json.NewDecoder` removed in v2**:
   - In `encoding/json/v2`, streaming is done via `json.MarshalWrite(io.Writer, v)` and `json.UnmarshalRead(io.Reader, &v)` (or syntactic encoders via `jsontext`).
   - Affected files:
     - `cmd/rest/handlerCollectionCreate.go` (line 23)
     - `cmd/rest/handlerItemCreate.go` (line 28)
     - `pkg/zotero/client/client_test.go` (line 208)
     - `pkg/zotero/client/cloud_api_test.go` (lines 596, 612, 616, 644, 654, 670, 674, 701, 711, 884, 910, 940, 1045, 1065, 1073, 1105, 1165, 1174)
     - `pkg/zotero/client/local_api_test.go` (lines 529, 540, 568, 578, 698, 709, 736, 746, 916)
     - `pkg/zotero/sync/sync_mock_test.go` (lines 181, 265, 289, 388, 396, 432, 511, 522, 782, 796)

3. **`omitempty` vs `omitzero` tag semantics in v2**:
   - In v1, `omitempty` omitted `0` for numbers. In v2, `omitempty` only omits empty JSON values (null, `""`, `{}`, `[]`), while Go zero values (such as `0` for integer fields) require `omitzero`.
   - Affected files:
     - `pkg/zotero/model/collection.go` (`CollectionData.Version` -> `json:"version,omitzero"`)
     - `pkg/zotero/model/item.go` (`ItemDataBase.Version` -> `json:"version,omitzero"`)

4. **Compiler warning in `pkg/filesystem/S3Fs.go`**:
   - Line 40: `return fmt.Sprintf(fs.s3.EndpointURL().String())` uses a non-constant format string.
   - Solution: `return fs.s3.EndpointURL().String()`.

### Key Decisions
- **Use `json.MarshalWrite` and `json.UnmarshalRead`**: Instead of instantiating raw `jsontext` encoders/decoders directly in REST handlers and mock tests, semantic marshaling/unmarshaling directly via `json.MarshalWrite` / `json.UnmarshalRead` is idiomatic in `encoding/json/v2`.
- **Use `omitzero` on `Version` fields**: Add `omitzero` to struct tags where zero version integers must not appear in generated JSON payloads.

# Testing

### Validation Approach
Verification will be done using the Go toolchain via CLI commands:

### Key Scenarios
1. **Compilation Check**:
   - `go build ./cmd/... ./pkg/...` must compile without errors or format string warnings.
2. **Model Unit Tests**:
   - `go test -v ./pkg/zotero/model/...` passes, confirming `TestCollectionDataVersionSerialization` and `TestItemGenericVersionSerialization` properly omit zero version numbers.
3. **Client and Storage Tests**:
   - `go test -v ./pkg/zotero/client/...` and `go test -v ./pkg/zotero/storage/...` pass with all HTTP mock handlers functioning.
4. **Sync and REST Tests**:
   - `go test -v ./pkg/zotero/sync/...` and `go test -v ./pkg/filesystem/...` pass.
5. **Full Project Test Suite**:
   - `go test ./...` completes with all packages reporting `ok`.

# Delivery Steps

### ✓ Step 1: Migrate JSON v2 API usage in storage, syncer, REST handlers, and test mocks
All occurrences of obsolete `encoding/json` v1 APIs (`MarshalIndent`, `NewEncoder`, `NewDecoder`) are replaced with `encoding/json/v2` and `encoding/json/jsontext` equivalents across production code and test suites.

- In `pkg/zotero/storage/group.go` and `pkg/zotero/sync/syncer.go`, import `encoding/json/jsontext` and replace `json.MarshalIndent(data, "", "  ")` with `json.Marshal(data, jsontext.WithIndent("  "))`.
- In `cmd/rest/handlerCollectionCreate.go` and `cmd/rest/handlerItemCreate.go`, replace `json.NewDecoder(r.Body).Decode(...)` with `json.UnmarshalRead(r.Body, ...)`.
- In test files (`pkg/zotero/client/client_test.go`, `pkg/zotero/client/cloud_api_test.go`, `pkg/zotero/client/local_api_test.go`, and `pkg/zotero/sync/sync_mock_test.go`), replace `json.NewEncoder(w).Encode(data)` with `json.MarshalWrite(w, data)` and `json.NewDecoder(r.Body).Decode(&data)` with `json.UnmarshalRead(r.Body, &data)`.

### ✓ Step 2: Fix model struct tags for v2 zero-value omission and resolve filesystem compile error
All unit tests pass and the entire project builds without compiler or linter errors.

- In `pkg/zotero/model/collection.go` (`CollectionData`) and `pkg/zotero/model/item.go` (`ItemDataBase`), change `json:"version,omitempty"` to `json:"version,omitzero"` (or `json:"version,omitzero,omitempty"`) so that integer `0` is properly omitted when marshaling under `encoding/json/v2`.
- In `pkg/filesystem/S3Fs.go`, update `fmt.Sprintf(fs.s3.EndpointURL().String())` to `fs.s3.EndpointURL().String()` to fix compiler warning/error on non-constant format string.
- Run `go build ./...` and `go test ./...` across all packages (`cmd/...`, `pkg/...`) to verify compilation and test suite success.