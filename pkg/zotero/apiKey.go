package zotero

import (
	"emperror.dev/errors"
	"encoding/json"
)

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

func (zot *Zotero) SetApiKey(apiKey string) {
	zot.apiKey = apiKey
	if zot.client != nil {
		if apiKey != "" {
			zot.client.SetAuthToken(apiKey)
			zot.client.SetHeader("Zotero-API-Key", apiKey)
		} else {
			zot.client.Header.Del("Zotero-API-Key")
		}
	}
}

func (zot *Zotero) AuthorizeLocal(appName string) (string, error) {
	if appName == "" {
		appName = "ZSync"
	}
	if zot.ServerId == "" {
		_, _ = zot.DetectServerId()
	}

	endpoint := "/local/authorize"
	if zot.Logger != nil {
		zot.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}

	payload := map[string]string{
		"appName": appName,
	}

	req := zot.client.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetBody(payload)
	if zot.ServerId != "" {
		req.SetHeader("Zotero-Server-ID", zot.ServerId)
	}

	resp, err := req.Post(endpoint)
	if err != nil {
		return "", errors.Wrapf(err, "failed to post to %s", endpoint)
	}
	if resp.StatusCode() != 200 {
		return "", errors.Errorf("local authorization failed at %s with status %d: %s", endpoint, resp.StatusCode(), resp.String())
	}

	var authResp LocalAuthResponse
	if err := json.Unmarshal(resp.Body(), &authResp); err != nil {
		return "", errors.Wrapf(err, "cannot unmarshal local auth response: %s", string(resp.Body()))
	}

	if authResp.Key != "" {
		zot.SetApiKey(authResp.Key)
		if zot.CurrentKey == nil {
			zot.CurrentKey = &ApiKey{
				UserId:   0,
				Username: "",
			}
		}
	}

	return authResp.Key, nil
}

func (zot *Zotero) getCurrentKey() (*ApiKey, error) {
	endpoint := "/keys/current"
	zot.Logger.Info().Msgf("rest call: %s", endpoint)

	resp, err := zot.client.R().
		SetHeader("Accept", "application/json").
		Get(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	if resp.StatusCode() != 200 {
		return nil, errors.Errorf("failed to get current key from %s: status %d (%s)", endpoint, resp.StatusCode(), resp.String())
	}
	rawBody := resp.Body()
	key := &ApiKey{}
	if err := json.Unmarshal(rawBody, key); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return key, nil
}
