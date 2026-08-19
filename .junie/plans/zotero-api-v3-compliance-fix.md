---
sessionId: session-260819-145600-61hp
---

# Requirements

### Overview & Goals
Review `pkg/zotero` against the official [Zotero Web API v3 specification](https://www.zotero.org/support/dev/web_api/v3/) and resolve discrepancies, structural bugs, URL routing errors, and missing schema features.

### Scope
- **In Scope**:
  - Verification of all endpoints, HTTP headers, status code handling, and query parameters used in `pkg/zotero`.
  - Fixing broken URL endpoint strings in `pkg/zotero/item.go` (upload and delete endpoints).
  - Fixing HTTP client configuration to include required `Zotero-API-Version: 3` headers.
  - Correcting inverted `CheckRetry` handling in `pkg/zotero/zotero.go`.
  - Fixing invalid struct tags in `pkg/zotero/group.go` and `pkg/zotero/item.go`.
  - Updating `ItemDataPerson` to support single-field creators (e.g., organizations/institutions).
  - Handling relation deserialization for both empty arrays and key-value maps.
  - Adding pagination to `group_tag.go` for tag synchronization.
- **Out of Scope**:
  - Modifying the PostgreSQL database schema (`syncgroups.sql`).
  - Rewriting CLI command tools outside `pkg/zotero`.

### Functional Requirements
- **API Versioning**: All requests must send `Zotero-API-Version: 3`.
- **Accurate Endpoint URLs**: File upload authorization (`POST /groups/<id>/items/<key>/file`), file registration (`POST /groups/<id>/items/<key>/file`), and item deletion (`DELETE /groups/<id>/items/<key>`) must target valid Zotero API routes.
- **Creator Compatibility**: Both 2-field creators (`firstName`, `lastName`) and 1-field creators (`name`) must be preserved during serialization and deserialization.
- **Tag Synchronization**: Fetching tags must iterate through all pages using `start` and `limit` until `Total-Results` is satisfied.
- **Backoff & Rate Limiting**: Honor `Backoff` and `Retry-After` (HTTP 429/503) headers correctly without entering infinite loops.

# Technical Design

### Current Implementation & Audit Findings

A comprehensive check of `pkg/zotero` against Zotero Web API v3 reveals several discrepancies and bugs:

#### 1. Missing HTTP Headers
- **Issue**: Zotero API v3 requires `Zotero-API-Version: 3` on all API calls. Currently `Init()` in `pkg/zotero/zotero.go` only configures authentication token.
- **Fix**: Set header `Zotero-API-Version: 3` on `zot.client`.

#### 2. Critical URL Path Bugs in `item.go`
- **Issue 1 (Upload Auth & Register)**: In `pkg/zotero/item.go` lines 291 and 379:
  ```go
  endpoint := fmt.Sprintf("/groups/%v/%v/items/file", item.Group.Id, item.Key)
  ```
  This produces `/groups/<id>/<key>/items/file` instead of `/groups/<id>/items/<key>/file`.
- **Issue 2 (Item Deletion)**: In `pkg/zotero/item.go` line 413:
  ```go
  endpoint := fmt.Sprintf("/groups/%v/%v/items", item.Group.Id, item.Key)
  ```
  This produces `/groups/<id>/<key>/items` instead of `/groups/<id>/items/<key>`.

#### 3. Inverted Retry Loop in `zotero.go`
- **Issue**: In `pkg/zotero/zotero.go` line 272:
  ```go
  if zot.CheckRetry(resp.Header()) {
      break
  }
  ```
  `CheckRetry` returns `true` when a `Retry-After` header was present (after sleeping). If no retry is needed (`false`), the loop continues infinitely.
- **Fix**: Invert condition to `if !zot.CheckRetry(resp.Header()) { break }` matching line 303.

#### 4. Invalid Struct Tags
- **Issue**: Missing quotes around tag values in Go structs:
  - `pkg/zotero/group.go` lines 24, 29, 30, 31: `json:owner`, `json:libraryEditing`, `json:libraryReading`, `json:fileEditing`
  - `pkg/zotero/group.go` line 60: `json:tagversion`
  - `pkg/zotero/item.go` line 78: `json:libraryid`
- **Fix**: Correct syntax to `json:"owner"`, `json:"libraryEditing"`, `json:"libraryReading"`, `json:"fileEditing"`, `json:"tagversion"`, and `json:"libraryid"`.

#### 5. Data Model Deficiencies
- **Creators (`ItemDataPerson`)**: Zotero API v3 supports single-field creators (e.g. `{"creatorType": "author", "name": "European Commission"}`). Currently `ItemDataPerson` only contains `FirstName` and `LastName`, causing data loss for institutional authors.
- **Relations**: Zotero API returns empty relations as `[]` or `{}`. `ItemDataBase.Relations` needs to handle both representations gracefully.

#### 6. Tag Retrieval & Pagination
- **Issue**: `GetTagsVersionCloud` in `pkg/zotero/group_tag.go` does not paginate and passes `since` query param, which `/tags` endpoint does not support for incremental version filtering in the same way as items.
- **Fix**: Implement pagination loop (`limit=100`, `start=0`) checking `Total-Results` header.

### Affected Files
- `pkg/zotero/zotero.go`: Client initialization, API version header, retry check logic.
- `pkg/zotero/item.go`: URL endpoints for upload/delete, `ItemDataPerson` struct definition, relation handling.
- `pkg/zotero/group.go`: Struct tag syntax corrections.
- `pkg/zotero/group_tag.go`: Pagination over `/tags` endpoint.
- `pkg/zotero/tag.go`: Struct tag validation.

# Testing

### Validation Approach
- Validate data contracts against official Zotero API v3 JSON schemas.
- Add unit tests in `pkg/zotero` testing serialization, deserialization, URL formatting, and header handling without requiring live external network credentials.

### Key Scenarios
- **Single-Field Creators**: Unmarshal item JSON with `{"creatorType": "author", "name": "Organization"}` and ensure `Name` is preserved upon re-serialization.
- **Two-Field Creators**: Unmarshal item JSON with `{"creatorType": "author", "firstName": "Ada", "lastName": "Lovelace"}` and verify `FirstName` and `LastName`.
- **Empty Relations**: Unmarshal items with both `"relations": {}` and `"relations": []` without error.
- **Endpoint Construction**: Assert that file upload and delete URLs match `/groups/{id}/items/{key}/file` and `/groups/{id}/items/{key}`.
- **Tag Pagination**: Validate that multiple pages of tags are combined when `Total-Results` exceeds page limit.
- **Rate Limit & Backoff**: Verify `CheckRetry` and `CheckBackoff` handle presence and absence of headers correctly.

# Delivery Steps

### ✓ Step 1: Fix API client configuration, retry logic, and URL routing bugs
Zotero API client sends standard v3 headers and all item, file upload, and deletion endpoints use correct URL paths with proper retry handling.

- Add the `Zotero-API-Version: 3` header in `zotero.Init()` default client configuration.
- Fix the inverted `CheckRetry` logic in `zotero.go` line 272 to prevent infinite loops or premature exits on 429 status codes.
- Fix URL formatting in `pkg/zotero/item.go` for `uploadFileCloud` (lines 291, 379: `/groups/%v/items/%s/file`) and `UpdateCloud` (line 413: `/groups/%v/items/%s`).

### ✓ Step 2: Correct struct tags and JSON models for Zotero v3 compliance
All Go struct tags and data structures accurately reflect Zotero Web API v3 payloads including institutional creators and flexible relations.

- Fix malformed struct tags in `pkg/zotero/group.go` (lines 24, 29-31, 60) and `pkg/zotero/item.go` (line 78) by adding proper quotes.
- Update `ItemDataPerson` in `pkg/zotero/item.go` to include `Name string `json:"name,omitempty"`` for single-field/institutional creators.
- Adapt `ItemDataBase.Relations` to safely deserialize both empty JSON arrays `[]` and relation maps `{}` returned by Zotero API v3.

### ✓ Step 3: Implement tag pagination and package unit tests
Tag fetching supports full pagination and comprehensive unit tests validate Zotero v3 serialization and endpoint construction.

- Update `GetTagsVersionCloud` / `SyncTags` in `pkg/zotero/group_tag.go` to handle pagination with `start`, `limit` (max 100), and `Total-Results` headers instead of non-supported `since` filters.
- Add unit test cases in `pkg/zotero/zotero_test.go` verifying URL generation, creator JSON parsing (single and two-field), relation parsing, and retry header handling.