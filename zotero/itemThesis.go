package zotero

type ItemDataThesis struct {
	ItemDataBase
	Title string `json:"title"`
	Creators        []ItemDataPerson `json:"creators"`
	AbstractNote    string           `json:"abstractNote"`
	ThesisType      string           `json:"thesisType"`
	University      string           `json:"university"`
	Place           string           `json:"place"`
	Date            string           `json:"date"`
	NumPages        string           `json:"numPages"`
	Language        string           `json:"language"`
	ShortTitle      string           `json:"shortTitle"`
	Url             string           `json:"url"`
	AccessData      string           `json:"accessDate"`
	Archive         string           `json:"archive"`
	ArchiveLocation string           `json:"archiveLocation"`
	LibraryCatalog  string           `json:"libraryCatalog"`
	CallNumber      string           `json:"callNumber"`
	Rights          string           `json:"rights"`
	Extra           string           `json:"extra"`
	Collections     []string         `json:"collections"`
}
