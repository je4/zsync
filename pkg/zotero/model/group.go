package model

import "time"

type GroupMeta struct {
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	NumItems     int64     `json:"numItems"`
}

type GroupData struct {
	Id             int64   `json:"id"`
	Version        int64   `json:"version"`
	Name           string  `json:"name"`
	Owner          int64   `json:"owner"`
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	Url            string  `json:"url"`
	HasImage       int64   `json:"hasImage"`
	LibraryEditing string  `json:"libraryEditing"`
	LibraryReading string  `json:"libraryReading"`
	FileEditing    string  `json:"fileEditing"`
	Admins         []int64 `json:"admins"`
}

// Group is the synchronization root and stores the cursors for its child
// collections, items, and tags.
type Group struct {
	Id                int64               `json:"id"`
	Version           int64               `json:"version"`
	Links             any                 `json:"links,omitempty"`
	Meta              GroupMeta           `json:"meta"`
	Data              GroupData           `json:"data"`
	Deleted           bool                `json:"-"`
	Active            bool                `json:"-"`
	SyncTags          bool                `json:"-"` // sync tags?
	Direction         SyncDirection       `json:"-"`
	ItemVersion       int64               `json:"-"`
	CollectionVersion int64               `json:"-"`
	TagVersion        int64               `json:"-"`
	IsModified        bool                `json:"-"`
	Gitlab            *time.Time          `json:"-"`
	Folder            string              `json:"-"`
	MetaMap           map[string][]string `json:"-"`
}

type GroupGitlab struct {
	Id                int64     `json:"id"`
	Data              GroupData `json:"data"`
	CollectionVersion int64     `json:"collectionversion"`
	ItemVersion       int64     `json:"itemversion"`
	TagVersion        int64     `json:"tagversion"`
}

// Init parses metadata embedded in the group's description.
func (group *Group) Init() {
	group.MetaMap = Text2Metadata(group.Data.Description)
}

// CanUpload reports whether the configured direction permits local-to-cloud
// synchronization.
func (group *Group) CanUpload() bool {
	return group.Direction == SyncDirection_BothCloud ||
		group.Direction == SyncDirection_BothLocal ||
		group.Direction == SyncDirection_BothManual ||
		group.Direction == SyncDirection_ToCloud
}

// CanDownload reports whether the configured direction permits cloud-to-local
// synchronization.
func (group *Group) CanDownload() bool {
	return group.Direction == SyncDirection_BothCloud ||
		group.Direction == SyncDirection_BothLocal ||
		group.Direction == SyncDirection_BothManual ||
		group.Direction == SyncDirection_ToLocal
}
