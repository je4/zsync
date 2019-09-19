package zotero

type ItemDataAttachment struct {
	ItemDataBase
	Title       string `json:"title"`
	LinkMode    string `json:"linkMode"`
	AccessDate  string `json:"accessDate"`
	Url         string `json:"url"`
	Note        string `json:"note"`
	ContentType string `json:"contentType"`
	Charset     string `json:"charset"`
	Filename    string `json:"filename"`
	MD5         string `json:"md5,omitempty"`
	MTime       int64  `json:"mtime"`
}

