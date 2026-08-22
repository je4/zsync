package storage

const (
	// Collection statements
	SQLInsertCollection                           = `INSERT INTO collections (key, version, library, sync, data, deleted) VALUES (@key, @version, @library, @sync, @data, false)`
	SQLRefreshCollectionNameHier                  = `REFRESH MATERIALIZED VIEW collection_name_hier WITH DATA`
	SQLInsertEmptyCollection                      = `INSERT INTO collections (key, version, library, sync) VALUES (@key, 0, @library, @sync)`
	SQLGetCollectionVersion                       = `SELECT version, sync FROM collections WHERE library = @library AND key = @key`
	SQLGetCollections                             = `SELECT key, version, data, meta, deleted, sync, gitlab FROM collections WHERE library = @library AND key = ANY(@keys)`
	SQLGetCollectionVersions                      = `SELECT key, version FROM collections WHERE library = @library AND version > @sinceVersion`
	SQLGetCollectionByKey                         = `SELECT cs.key, cs.version, cs.data, cs.meta, cs.deleted, cs.sync, cs.gitlab FROM collections cs WHERE cs.library = @library AND cs.key = @key`
	SQLGetCollectionByName                        = `SELECT cs.key, cs.version, cs.data, cs.meta, cs.deleted, cs.sync, cs.gitlab FROM collections cs, collection_name_hier cnh WHERE cs.key = cnh.key AND cs.library = @library AND cnh.name = @name AND cnh.parent = @parent`
	SQLGetCollectionByNameTop                     = `SELECT cs.key, cs.version, cs.data, cs.meta, cs.deleted, cs.sync, cs.gitlab FROM collections cs, collection_name_hier cnh WHERE cs.key = cnh.key AND cs.library = @library AND cnh.name = @name AND cnh.parent IS NULL`
	SQLUpdateCollection                           = `UPDATE collections SET version = @version, sync = @sync, data = @data, meta = @meta, deleted = @deleted, modified = NOW() WHERE library = @library AND key = @key`
	SQLDeleteCollection                           = `UPDATE collections SET deleted = true, sync = @sync, modified = NOW() WHERE library = @library AND key = @key`
	SQLDeleteCollections                          = `UPDATE collections SET deleted = true, sync = @sync, modified = NOW() WHERE library = @library AND key = ANY(@keys)`
	SQLIterateCollectionsCount                    = `SELECT COUNT(*) FROM collections WHERE library = @library AND deleted = false`
	SQLIterateCollectionsAfterCount               = `SELECT COUNT(*) FROM collections WHERE library = @library AND deleted = false AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLIterateCollections                         = `SELECT key, version, data, meta, deleted, sync, gitlab FROM collections WHERE library = @library AND deleted = false`
	SQLIterateCollectionsAfter                    = `SELECT key, version, data, meta, deleted, sync, gitlab FROM collections WHERE library = @library AND deleted = false AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLIterateCollectionsAllCount                 = `SELECT COUNT(*) FROM collections WHERE library = @library`
	SQLIterateCollectionsAllAfterCount            = `SELECT COUNT(*) FROM collections WHERE library = @library AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLIterateCollectionsAll                      = `SELECT key, version, data, meta, deleted, sync, gitlab FROM collections WHERE library = @library`
	SQLIterateCollectionsAllAfter                 = `SELECT key, version, data, meta, deleted, sync, gitlab FROM collections WHERE library = @library AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLGetModifiedCollections                     = `SELECT key, version, data, meta, deleted, sync, gitlab FROM collections WHERE library = @library AND (sync = @syncNew OR sync = @syncModified)`
	SQLUpdateCollectionsGitlabTimestamp           = `UPDATE collections SET gitlab = TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') WHERE library = @library AND (TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') > gitlab OR gitlab IS NULL)`
	SQLUpdateCollectionsGitlabTimestampWithFilter = `UPDATE collections SET gitlab = TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') WHERE library = @library AND (TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') > gitlab OR gitlab IS NULL) AND (gitlab >= TO_TIMESTAMP(@gitlab, 'YYYY-MM-DD HH24:MI:SS') OR gitlab IS NULL)`

	// Group statements
	SQLGetGroup                    = `SELECT version, created, modified, data, active, direction, tags, itemversion, collectionversion, tagversion, gitlab FROM groups g, syncgroups sg WHERE g.id = sg.id AND g.id = @id`
	SQLGetGroups                   = `SELECT id FROM syncgroups sg WHERE sg.active = true`
	SQLInsertEmptyGroup            = `INSERT INTO groups (id, version, created, modified) VALUES (@id, 0, NOW(), NOW())`
	SQLInsertEmptySyncGroup        = `INSERT INTO syncgroups (id, active, direction) VALUES (@id, @active, @direction)`
	SQLGetSyncGroupActiveDirection = `SELECT active, direction FROM syncgroups WHERE id = @id`
	SQLClearGroup                  = `UPDATE groups SET version = 0, modified = created, itemversion = 0, collectionversion = 0 WHERE id = @id`
	SQLUpdateGroup                 = `UPDATE groups SET version = @version, created = @created, modified = @modified, data = @data, deleted = @deleted, itemversion = @itemversion, collectionversion = @collectionversion, tagversion = @tagversion WHERE id = @id`
	SQLUpdateGroupGitlabTimestamp  = `UPDATE groups SET gitlab = TO_TIMESTAMP(@gitlab, 'YYYY-MM-DD HH24:MI:SS') WHERE id = @id`

	// Item statements
	SQLInsertItem                           = `INSERT INTO items (key, version, library, sync, data, oldid) VALUES (@key, @version, @library, @sync, @data, @oldid)`
	SQLInsertEmptyItem                      = `INSERT INTO items (key, version, library, sync, oldid) VALUES (@key, 0, @library, @sync, @oldid)`
	SQLGetItemVersion                       = `SELECT version, sync FROM items WHERE library = @library AND key = @key`
	SQLGetItemVersions                      = `SELECT key, version FROM items WHERE library = @library AND version > @sinceVersion AND trashed = @trashed`
	SQLGetItems                             = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND key = ANY(@keys)`
	SQLGetItemByKey                         = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND key = @key`
	SQLGetItemByOldid                       = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND oldid = @oldid`
	SQLUpdateItem                           = `UPDATE items SET version = @version, data = @data, meta = @meta, trashed = @trashed, deleted = @deleted, sync = @sync, md5 = @md5, modified = NOW() WHERE library = @library AND key = @key`
	SQLUpdateItemVersion0                   = `UPDATE items SET data = @data, meta = @meta, trashed = @trashed, deleted = @deleted, sync = @sync, md5 = @md5, modified = NOW() WHERE library = @library AND key = @key`
	SQLDeleteItem                           = `UPDATE items SET deleted = true, sync = @sync, modified = NOW() WHERE key = @key AND library = @library`
	SQLDeleteItems                          = `UPDATE items SET deleted = true, sync = @sync, modified = NOW() WHERE library = @library AND key = ANY(@keys)`
	SQLGetChildren                          = `SELECT i.key, i.version, i.data, i.meta, i.trashed, i.deleted, i.sync, i.md5, i.gitlab FROM items i, item_type_hier ith WHERE i.trashed = false AND i.deleted = false AND i.key = ith.key AND i.library = ith.library AND i.library = @library AND ith.parent = @parent`
	SQLIterateItemsCount                    = `SELECT COUNT(*) FROM items WHERE library = @library AND deleted = false`
	SQLIterateItemsAfterCount               = `SELECT COUNT(*) FROM items WHERE library = @library AND deleted = false AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLIterateItems                         = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND deleted = false`
	SQLIterateItemsAfter                    = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND deleted = false AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLIterateItemsAllCount                 = `SELECT COUNT(*) FROM items WHERE library = @library`
	SQLIterateItemsAllAfterCount            = `SELECT COUNT(*) FROM items WHERE library = @library AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLIterateItemsAll                      = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library`
	SQLIterateItemsAllAfter                 = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND (modified > TO_TIMESTAMP(@after, 'YYYY-MM-DD HH24:MI:SS'))`
	SQLGetModifiedItems                     = `SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM items WHERE library = @library AND (sync = @syncNew OR sync = @syncModified)`
	SQLRefreshItemTypeHier                  = `SELECT refresh_item_type_hier()`
	SQLUpdateItemsGitlabTimestamp           = `UPDATE items SET gitlab = TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') WHERE library = @library AND (TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') > gitlab OR gitlab IS NULL)`
	SQLUpdateItemsGitlabTimestampWithFilter = `UPDATE items SET gitlab = TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') WHERE library = @library AND (TO_TIMESTAMP(@now, 'YYYY-MM-DD HH24:MI:SS') > gitlab OR gitlab IS NULL) AND (gitlab >= TO_TIMESTAMP(@gitlab, 'YYYY-MM-DD HH24:MI:SS') OR gitlab IS NULL)`

	// Tag statements
	SQLInsertTag  = `INSERT INTO tags (tag, meta, library) VALUES (@tag, @meta, @library)`
	SQLDeleteTag  = `DELETE FROM tags WHERE tag = @tag AND library = @library`
	SQLDeleteTags = `DELETE FROM tags WHERE library = @library AND tag = ANY(@tags)`
)
