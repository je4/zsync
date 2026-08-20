package zotero

import (
	"emperror.dev/errors"
	"encoding/json"
	"fmt"
	"strconv"
)

type User struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Links    any    `json:"links,omitempty"`
}

func (zot *Zotero) GetUserGroupVersions(key *ApiKey) (*map[int64]int64, error) {
	var endpoint string
	if key != nil && key.UserId != 0 {
		endpoint = fmt.Sprintf("/users/%v/groups", key.UserId)
	} else if zot.CurrentKey != nil && zot.CurrentKey.UserId != 0 {
		endpoint = fmt.Sprintf("/users/%v/groups", zot.CurrentKey.UserId)
	} else {
		endpoint = "/users/0/groups"
	}
	if zot.Logger != nil {
		zot.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	resp, err := zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("format", "versions").
		Get(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get group versions from %s", endpoint)
	}
	if resp.IsError() {
		return nil, errors.Errorf("cannot get user group versions from %s: status %d - %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}
	rawBody := resp.Body()
	groups := map[string]int64{}
	if err := json.Unmarshal(rawBody, &groups); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	res := make(map[int64]int64)
	for gId, version := range groups {
		id, err := strconv.ParseInt(gId, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot parse %s", gId)
		}
		res[id] = version
	}
	return &res, nil
}
