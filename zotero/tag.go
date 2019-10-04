package zotero

type TagMeta struct {
	Type     int64 `json:"type"`
	NumItems int64 `json:"numItems"`
}

type Tag struct {
	Tag   string      `json:"tag"`
	Links interface{} `json:"links,omitempty"`
	Meta  *TagMeta    `json:"meta,omitempty"`
}

