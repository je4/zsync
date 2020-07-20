package zotero

type ItemDataNote struct {
	ItemDataBase
	Note       string `json:"note"`
}
