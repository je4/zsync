package client

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strconv"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

// GetDeleted returns the keys of all objects deleted from the given group since
// sinceVersion. A group without any deletions yields an empty, non-nil result.
func (c *Client) GetDeleted(ctx context.Context, groupId int64, sinceVersion int64) (*model.Delete, error) {
	endpoint := fmt.Sprintf("/groups/%v/deleted", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(sinceVersion, 10))

	resp, err := call.Get(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get deleted from %s", endpoint)
	}
	if resp.StatusCode() == 404 {
		return emptyDelete(), nil
	}
	if resp.StatusCode() >= 400 {
		return nil, errors.Errorf("failed to get deleted from %s with status %d: %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}
	rawBody := resp.Body()
	del := emptyDelete()
	if err := json.Unmarshal(rawBody, del); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	c.CheckBackoff(resp.Header())

	return del, nil
}

func emptyDelete() *model.Delete {
	return &model.Delete{
		Collections: []string{},
		Searches:    []string{},
		Items:       []string{},
		Tags:        []string{},
	}
}
