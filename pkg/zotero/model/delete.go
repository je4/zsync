package model

type Delete struct {
	Collections []string `json:"collections"`
	Searches    []string `json:"searches"`
	Items       []string `json:"items"`
	Tags        []string `json:"tags"`
}
