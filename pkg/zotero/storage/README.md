# `storage`

The storage package persists Zotero data in PostgreSQL through a
`pgxpool.Pool`. It is organized by aggregate: `group.go`, `collection.go`,
`item.go`, and `tag.go`. `storageStatements.go` contains the SQL constants.

## Responsibilities

- map database rows to `model` objects and back;
- retain sync status, trash/deletion flags, cursors, MD5 values, and backup timestamps;
- provide key, name, version, iteration, and modified-record queries;
- mark deletions instead of immediately removing items and collections;
- refresh collection-name and item-type hierarchy helpers after changes.

`CreateEmpty*` methods support a two-phase import. `GetModifiedItems` and
`GetModifiedCollections` are the upload queues consumed by `sync.Syncer`.

The package expects a PostgreSQL pool and uses named query arguments.
`IsEmptyResult` and `IsUniqueViolation` normalize common database errors.
