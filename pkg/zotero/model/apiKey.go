package model

type AccessElements struct {
	Library bool `json:"library,omitempty"`
	Files   bool `json:"files,omitempty"`
	Notes   bool `json:"notes,omitempty"`
	Write   bool `json:"write,omitempty"`
}

type Access struct {
	User   AccessElements            `json:"user"`
	Groups map[string]AccessElements `json:"groups,omitempty"`
}

type ApiKey struct {
	UserId   int64  `json:"userId"`
	Username string `json:"username"`
	Access   Access `json:"access"`
}

type LocalAuthResponse struct {
	Key      string `json:"key"`
	Remember bool   `json:"remember"`
}
