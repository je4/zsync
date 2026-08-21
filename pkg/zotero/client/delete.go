package client

import (
	"encoding/json"
	"fmt"
	"strconv"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func (c *Client) GetDeleted(groupId int64, sinceVersion int64) (collections *[]string, items *[]string, tags *[]string, err error) {
	endpoint := fmt.Sprintf("/groups/%v/deleted", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(sinceVersion, 10))

	resp, err := call.Get(endpoint)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "cannot get deleted from %s", endpoint)
	}
	if resp.StatusCode() == 404 {
		empty := []string{}
		return &empty, &empty, &empty, nil
	}
	if resp.StatusCode() >= 400 {
		return nil, nil, nil, errors.Errorf("failed to get deleted from %s with status %d: %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}
	rawBody := resp.Body()
	del := &model.Delete{}
	if err := json.Unmarshal(rawBody, del); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	c.CheckBackoff(resp.Header())

	collections = &del.Collections
	items = &del.Items
	tags = &del.Tags

	return
}
