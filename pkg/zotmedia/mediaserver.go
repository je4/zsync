package zotmedia

type Mediaserver interface {
	IsMediaserverURL(url string) (string, string, bool)
	GetCollectionByName(name string) (*Collection, error)
	GetCollectionById(id int64) (*Collection, error)
	CreateMasterUrl(collection, signature, url string) error
	GetMetadata(collection, signature string) (*Metadata, error)
}
