---
sessionId: session-260820-082621-1vhq
---

# Requirements

### Overview & Goals
Enable authorized write operations in local Zotero API tests and client by supporting the Zotero desktop local authorization handshake (`POST /api/local/authorize`). This allows `TestLocalApi_CreateAndRetainItems` and `TestLocalApi_CreateAndRetainCollections` to successfully authenticate, create, update, and retain test items and collections in the local Zotero `APITEST` group (`6642571`).

### Scope
- **In Scope:**
  - Implement `AuthorizeLocal(appName string) (string, error)` method on `Zotero` client in `pkg/zotero/apiKey.go` or `pkg/zotero/zotero.go`.
  - When initializing or writing to a local Zotero desktop endpoint (`localhost`/`127.0.0.1`), automatically perform local authorization if no valid API key is present or when receiving 401.
  - Update `pkg/zotero/zotero_local_api_test.go` to obtain local authorization for write operations against the local Zotero instance.
  - Verify write, update, and retention operations against the local `APITEST` library.
- **Out of Scope:**
  - Cloud server database schema changes or sync daemon refactoring.

### User Stories
- As a developer, I want the Zotero client and integration tests to automatically authorize against the local Zotero desktop API via `/api/local/authorize` so that write tests succeed and populate items/collections in the `APITEST` group.

# Technical Design

### Current Implementation
- Local Zotero Desktop connector API (`http://localhost:23119/api`) requires `Zotero-Server-ID` and a session key generated via `POST /api/local/authorize` with `{"appName": "..."}`.
- Passing arbitrary/expired cloud API keys returns `401 Unauthorized: Invalid or expired API key`.
- `TestLocalApi_CreateAndRetainItems` and `TestLocalApi_CreateAndRetainCollections` catch 401 and invoke `t.Skipf`, leaving `APITEST` with 0 items and 0 collections.

### Key Decisions
- **Local Handshake Support:** Add `AuthorizeLocal(appName string) (string, error)` in `pkg/zotero/apiKey.go`.
  - Sends `POST /api/local/authorize` with `Zotero-Server-ID` header and payload `{"appName": appName}`.
  - Stores returned `key` in `zot.apiKey`, updates `Zotero-API-Key` header and auth token on `zot.client`.
- **Automatic Local Client Init:** In `getTestClient()` or `zot.Init()`, detect local endpoints and call `AuthorizeLocal("ZSync")` when appropriate.

### Proposed Changes
- **`pkg/zotero/apiKey.go`:**
  - Add `type LocalAuthResponse struct { Key string json:"key", Remember bool json:"remember" }`.
  - Add `func (zot *Zotero) AuthorizeLocal(appName string) (string, error)`.
- **`pkg/zotero/zotero_local_api_test.go`:**
  - Update `getTestClient()` to invoke `zot.AuthorizeLocal("ZSyncTest")` for write capability against local Zotero.
  - Run and verify `TestLocalApi_CreateAndRetainItems`, `TestLocalApi_CreateAndRetainCollections`, and `TestLocalApi_VerifyRetainedData`.

# Testing

### Validation Approach
- Execute `go test -v -run TestLocalApi ./pkg/zotero/...`.
- Verify `TestLocalApi_CreateAndRetainItems` and `TestLocalApi_CreateAndRetainCollections` succeed without skipping.
- Verify `TestLocalApi_VerifyRetainedData` confirms created items and collections exist in local Zotero `APITEST` group (`6642571`).

# Delivery Steps

### ✓ Step 1: Implement Local Authorization Handshake in pkg/zotero
- Add `AuthorizeLocal(appName string) (string, error)` in `pkg/zotero/apiKey.go`.
- Ensure `Zotero-Server-ID` header and `Zotero-API-Key` header are properly set on successful authorization.

### ✓ Step 2: Wire Local Authorization in Integration Tests and Verify Data Creation
- Call `AuthorizeLocal` in `pkg/zotero/zotero_local_api_test.go` test setup.
- Run tests and verify that items and collections are successfully created and retained in local Zotero `APITEST` group.