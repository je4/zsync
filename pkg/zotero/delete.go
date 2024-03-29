package zotero

import (
	"emperror.dev/errors"
	"encoding/json"
	"fmt"
	"strconv"
)

type Delete struct {
	Collections []string `json:"collections"`
	Searches    []string `json:"searches"`
	Items       []string `json:"items"`
	Tags        []string `json:"tags"`
}

func (group *Group) GetDeleted(sinceVersion int64) (collections *[]string, items *[]string, tags *[]string, err error) {
	endpoint := fmt.Sprintf("/groups/%v/deleted", group.Id)
	group.Zot.Logger.Info().Msgf("rest call: %s", endpoint)

	resp, err := group.Zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
		Get(endpoint)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	delete := &Delete{}
	if err := json.Unmarshal(rawBody, delete); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}

	collections = &delete.Collections
	items = &delete.Items
	tags = &delete.Tags

	return
}
