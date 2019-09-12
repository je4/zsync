package zotero

type ItemDataNote struct {
	ItemDataBase
	ParentItem string `json:"parentItem"`
	Note       string `json:"note"`
}
