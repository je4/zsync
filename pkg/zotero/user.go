package zotero

import (
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"strconv"
)

type User struct {
	Id       int64       `json:"id"`
	Username string      `json:"username"`
	Links    interface{} `json:"links,omitempty"`
}

func (zot *Zotero) GetUserGroupVersions(key *ApiKey) (*map[int64]int64, error) {
	endpoint := fmt.Sprintf("/users/%v/groups", key.UserId)
	zot.Logger.Infof("rest call: %s", endpoint)

	resp, err := zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("format", "versions").
		Get(endpoint)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	groups := map[string]int64{}
	if err := json.Unmarshal(rawBody, &groups); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	result := &map[int64]int64{}
	for gId, version := range groups {
		id, err := strconv.ParseInt(gId, 10, 64)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot parse %s", gId)
		}
		(*result)[id] = version
	}
	return result, nil
}
