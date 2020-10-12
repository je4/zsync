package zotero

import (
	"encoding/json"
	"github.com/goph/emperror"
)

type AccessElements struct {
	Library bool `json:"library,omitempty"`
	Files   bool `json:"files,omitempty"`
	Notes   bool `json:"notes,omitempty"`
	Write   bool `json:"write,omitempty"`
}

type Access struct {
	User   AccessElements            `json:"user,omitempty"`
	Groups map[string]AccessElements `json:"groups,omitempty"`
}

type ApiKey struct {
	UserId   int64  `json:"userId"`
	Username string `json:"username"`
	Access   Access `json:"access"`
}

func (zot *Zotero) getCurrentKey() (*ApiKey, error) {
	endpoint := "/keys/current"
	zot.Logger.Infof("rest call: %s", endpoint)

	resp, err := zot.client.R().
		SetHeader("Accept", "application/json").
		Get(endpoint)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	key := &ApiKey{}
	if err := json.Unmarshal(rawBody, key); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return key, nil
}
