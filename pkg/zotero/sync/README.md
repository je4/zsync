# `sync`

The sync package contains synchronization policy. `Syncer` coordinates one
Zotero `model.Group` through `client.Client`, `storage.Storage`, and an optional
attachment `filesystem.FileSystem`.

## `SyncGroup` pipeline

```mermaid
sequenceDiagram
    participant S as Syncer
    participant D as Storage
    participant Z as Zotero Client
    participant F as Attachment FS
    S->>D: Read local group and cursors
    S->>Z: Upload modified collections and items
    Z-->>S: Keys and Last-Modified-Version
    S->>Z: Fetch changed version maps
    Z-->>S: Version maps
    S->>Z: Fetch changed objects (batches of 50)
    Z-->>S: Objects and tags
    S->>F: Transfer imported attachment files
    S->>D: Persist objects and cursors
    S->>Z: Fetch deleted-object feed
    S->>D: Mark deleted records
```

The public stages are `SyncCollections`, `UploadItems`, `DownloadItems`,
`SyncTags`, and `SyncDeleted`; `SyncGroup` runs them in that order. Stages are
skipped according to `Group.Direction`, and tags require `Group.SyncTags`.

`BackupLocal` writes group, collection, and item JSON below a group directory.
Imported attachment bytes are written as `<item-key>.bin`; backup timestamps
are stored in PostgreSQL for incremental backups.

Uploads use Zotero version preconditions. A failed precondition indicates that
the remote object changed and must not be overwritten blindly.
