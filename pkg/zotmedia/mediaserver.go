package zotmedia

type Mediaserver interface {
	IsMediaserverURL(url string) (string, string, bool)
	GetCollectionByName(name string)
	GetCollectionById(id int64)
	CreateMasterUrl(collection, signature, url string)
	GetMetadata(collection, signature string)
}
