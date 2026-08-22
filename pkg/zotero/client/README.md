# `client`

The client package is the Zotero HTTP adapter. `Client` configures Resty with
the endpoint, API key, Zotero API version, server ID, and user agent. Resource
methods are grouped by endpoint in `user.go`, `collection.go`, `item.go`,
`tag.go`, and `delete.go`.

## API groups

| File | Main operations |
| --- | --- |
| `user.go` | Read groups and the user's group version map. |
| `collection.go` | Read versions, query/read collections, create, update, and delete. |
| `item.go` | Read versions/items, create, update, delete, and transfer attachments. |
| `tag.go` | Read changed tags. |
| `delete.go` | Read the deleted-object feed. |
| `paginator.go` | Shared pagination for slices and version maps. |

Most methods accept a `context.Context`, set Zotero headers, decode JSON into
`model` values, and return wrapped errors. Pagination uses the API's
`limit`/`start` convention. `Retry-After` is honored before retrying and
`Backoff` delays subsequent work. `Last-Modified-Version` is returned as a
cursor to the synchronizer.

`AuthorizeLocal` is different from cloud-key setup: it requests interactive
permission from the Zotero desktop application's local HTTP server and stores
the returned local token on the client.

Attachment upload is a three-stage protocol: request authorization, upload the
bytes to the returned object-storage URL, and register the upload with Zotero.
`DownloadAttachment` returns bytes, content type, and an MD5/ETag value.
