package model

import "time"

type CollectionData struct {
	Key              string       `json:"key,omitempty"`
	Name             string       `json:"name"`
	Version          int64        `json:"version,omitzero"`
	Relations        RelationList `json:"relations"`
	ParentCollection Parent       `json:"parentCollection"`
}

type CollectionMeta struct {
	NumCollections int64 `json:"numCollections"`
	NumItems       int64 `json:"numItems"`
}

type Collection struct {
	Key     string         `json:"key"`
	Version int64          `json:"version"`
	Library Library        `json:"library"`
	Links   any            `json:"links,omitempty"`
	Meta    CollectionMeta `json:"meta"`
	Data    CollectionData `json:"data"`
	Status  SyncStatus     `json:"-"`
	Trashed bool           `json:"-"`
	Deleted bool           `json:"-"`
	Gitlab  *time.Time     `json:"-"`
}

type CollectionGitlab struct {
	LibraryId int64          `json:"libraryid"`
	Key       string         `json:"key"`
	Data      CollectionData `json:"data"`
	Meta      CollectionMeta `json:"meta"`
}
