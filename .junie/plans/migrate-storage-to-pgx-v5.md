---
sessionId: session-260820-165341-v8vd
---

# Requirements

### Overview & Goals
Enhance the testing suite for `pkg/zotero/storage`:
1. Ensure integration tests gracefully skip (`t.Skip`) when database connectivity fails (e.g. `pool.Ping` fails or database is unreachable), rather than failing the test suite.
2. Implement a standalone mock-based test suite using `github.com/jackc/pgmock` to validate `Storage` operations (query generation, response parsing, error handling) completely offline without requiring a live PostgreSQL instance.

### Scope
- **In Scope**:
  - Updating `getTestStorage` in `pkg/zotero/storage/storage_test.go` to call `t.Skipf` when connection pool creation, `pool.Ping()`, or schema initialization fails due to connection issues.
  - Adding `github.com/jackc/pgmock` (and `github.com/jackc/pgproto3/v2`) to `go.mod`.
  - Implementing dedicated mock tests in `pkg/zotero/storage/storage_mock_test.go` using `pgmock.Script` and `pgproto3` message flows to test `Storage` methods (e.g. `LoadGroup`, `GetCollectionByKey`, `GetItemByKey`, `CreateTag`, and constraint error scenarios).
- **Out of Scope**:
  - Modifying the underlying `Storage` business logic or API contracts (already migrated to `pgx/v5`).
  - Replacing live integration tests (both mock tests and live integration tests will coexist).

### User Stories
- **As a Developer / CI Pipeline**, I want storage integration tests to automatically skip when no database is running or reachable so that builds and CI workflows do not fail due to missing database infrastructure.
- **As a Developer**, I want isolated unit tests using `jackc/pgmock` so that I can reliably test SQL interaction, protocol handling, and edge cases for `Storage` in fast, hermetic test runs.

### Functional Requirements
- **FR-1**: If `DATABASE_URL` is empty or if connecting/pinging the database fails in `getTestStorage`, the test must call `t.Skip(...)` (not `t.Fatalf`).
- **FR-2**: Mock tests using `jackc/pgmock` must spin up a local TCP listener, serve simulated PostgreSQL protocol messages (startup handshake, query parse/bind/execute, row descriptions, data rows, command complete, error responses), and verify `Storage` behaviors without requiring an actual PostgreSQL server.
- **FR-3**: Mock tests must cover key CRUD and query workflows such as `LoadGroup`, `GetCollectionByKey`, `GetItemByKey`, and error responses (e.g. `pgconn.PgError` unique violation handling).

### Non-Functional Requirements
- **Hermetic & Fast**: `pgmock` tests must execute in milliseconds without external network dependencies.
- **Compatibility**: Ensure full project compilation and test execution (`go test ./...` and `go build ./...`).

# Technical Design

### Current Implementation
- `pkg/zotero/storage/storage_test.go` defines integration tests (`TestIntegration_GroupLifecycle`, `TestIntegration_CollectionLifecycle`, `TestIntegration_ItemLifecycle`, `TestIntegration_TagLifecycle`) that use `getTestStorage(t)`.
- If `DATABASE_URL` is empty, `t.Skip` is called; however, if `DATABASE_URL` is provided but the host is unreachable, `pool.Ping(ctx)` fails with `t.Fatalf`, causing the test run to fail.

### Key Decisions
- **Graceful Skip on Unreachable DB**: Wrap connection and ping checks in `getTestStorage` with `t.Skipf(...)` so that when a database server is not reachable, integration tests are skipped rather than failing.
- **Mock Server with `jackc/pgmock`**: Use `github.com/jackc/pgmock` and `github.com/jackc/pgproto3/v2` to simulate a PostgreSQL server over an in-process TCP listener (`net.Listen("tcp", "127.0.0.1:0")`).
- **Protocol Simulation**: Use `pgmock.AcceptUnauthenticatedConnRequestSteps()` for connection handshake and define `pgmock.Step` sequences (expecting frontend queries/extended protocol messages and sending backend `RowDescription`, `DataRow`, `CommandComplete`, and `ReadyForQuery` or `ErrorResponse`).
- **Connection to Mock Server**: Instantiate `pgxpool.New` with connection string targeting the mock listener (`postgres://user:pass@127.0.0.1:<port>/zotero?sslmode=disable`).

### Proposed Changes

#### 1. Integration Test Ping Skip (`pkg/zotero/storage/storage_test.go`)
- Update `getTestStorage`:
  ```go
  pool, err := pgxpool.New(ctx, dbURL)
  if err != nil {
      t.Skipf("cannot create connection pool (%v); skipping integration tests", err)
      return nil, 0, nil
  }
  if err := pool.Ping(ctx); err != nil {
      pool.Close()
      t.Skipf("cannot ping database (%v); skipping integration tests", err)
      return nil, 0, nil
  }
  if err := ensureSchema(ctx, pool, schema); err != nil {
      pool.Close()
      t.Skipf("cannot ensure schema (%v); skipping integration tests", err)
      return nil, 0, nil
  }
  ```

#### 2. Mock Test Suite (`pkg/zotero/storage/storage_mock_test.go`)
- Implement a helper to start a mock PostgreSQL server:
  ```go
  func startMockServer(t *testing.T, script *pgmock.Script) (*Storage, func())
  ```
- Implement test cases:
  - `TestPgMock_LoadGroup`: Simulates `SELECT version, created, modified, data, active... FROM public.groups...` returning row data, verifying `LoadGroup` parses group fields properly.
  - `TestPgMock_GetCollectionByKey`: Simulates querying `public.collections` by key and verifies model deserialization.
  - `TestPgMock_GetItemByKey`: Simulates querying `public.items` by key and verifies model deserialization.
  - `TestPgMock_IsUniqueViolation`: Simulates returning a PostgreSQL `ErrorResponse` (Code `23505`, Constraint `items_oldid_constraint`) and verifies error handling in `Storage`.

### Architecture Diagram
```mermaid
graph TD
    subgraph MockTest [Mock Testing Suite: storage_mock_test.go]
        TestRunner[TestRunner]
        PgMockServer[pgmock Server / TCP Listener]
        PgxPool[pgxpool.Pool]
        StorageInstance[storage.Storage]
    end

    TestRunner -->|Starts| PgMockServer
    TestRunner -->|Connects to 127.0.0.1:port| PgxPool
    PgxPool --> StorageInstance
    StorageInstance -->|pgx queries / extended protocol| PgMockServer
    PgMockServer -->|pgproto3 Backend Messages| StorageInstance
```

### File Structure & Changes
- Modified: `pkg/zotero/storage/storage_test.go` (graceful skip on ping/connect failure)
- Added: `pkg/zotero/storage/storage_mock_test.go` (pgmock unit tests)
- Modified: `go.mod`, `go.sum` (add `github.com/jackc/pgmock` dependency)

# Testing

### Validation Approach
- **Ping Skip Verification**: Run tests with invalid `DATABASE_URL` (e.g. `postgres://localhost:54329/nonexistent`) to verify that tests are gracefully skipped (`SKIP`) instead of failing (`FAIL`).
- **PgMock Unit Tests**: Run `go test -v -run TestPgMock ./pkg/zotero/storage/...` to verify all mock server tests pass without any live database.
- **Full Test Suite**: Execute `go test ./...` and `go build ./...` across the entire workspace.

### Key Scenarios
- **Unreachable Database**: `getTestStorage` calls `t.Skipf` when `pool.Ping` fails, allowing test suite to pass.
- **Mock LoadGroup**: `Storage.LoadGroup(groupId)` correctly issues query to mock server and constructs `model.Group`.
- **Mock GetCollectionByKey**: `Storage.GetCollectionByKey(groupId, key)` correctly receives mocked `DataRow` and parses metadata and JSON data.
- **Mock GetItemByKey**: `Storage.GetItemByKey(groupId, key)` correctly parses item properties and metadata from mocked responses.
- **Mock Error Handling**: Server error responses (code `23505`) are correctly propagated and identified as unique violations.

# Delivery Steps

### ✓ Step 1: Update integration test helper to skip on database ping / connection failure
The test helper in `pkg/zotero/storage/storage_test.go` skips tests gracefully when database connection or ping fails.

- Update `getTestStorage(t *testing.T)` in `pkg/zotero/storage/storage_test.go` to replace `t.Fatalf` with `t.Skipf` on `pgxpool.New`, `pool.Ping`, and `ensureSchema` errors.
- Ensure that if a database is not running or ping times out, all integration tests report as skipped rather than failing.

### ✓ Step 2: Implement mock-based test suite using jackc/pgmock
A new test suite in `pkg/zotero/storage/storage_mock_test.go` validates `Storage` using `github.com/jackc/pgmock` without requiring a real PostgreSQL instance.

- Add `github.com/jackc/pgmock` to dependencies in `go.mod` and run `go mod tidy`.
- Create `pkg/zotero/storage/storage_mock_test.go` with mock server lifecycle helper using `net.Listen` and `pgmock.Script`.
- Implement mock test cases for `LoadGroup`, `GetCollectionByKey`, `GetItemByKey`, and constraint violation handling.
- Verify tests pass with `go test -v ./pkg/zotero/storage/...` and workspace build passes with `go build ./...`.