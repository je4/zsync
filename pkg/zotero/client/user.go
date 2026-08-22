package client

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strconv"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

func (c *Client) GetUserGroupVersions(ctx context.Context, key *model.ApiKey) (map[int64]int64, error) {
	var endpoint string
	if key != nil && key.UserId != 0 {
		endpoint = fmt.Sprintf("/users/%v/groups", key.UserId)
	} else if c.CurrentKey != nil && c.CurrentKey.UserId != 0 {
		endpoint = fmt.Sprintf("/users/%v/groups", c.CurrentKey.UserId)
	} else {
		endpoint = "/users/0/groups"
	}
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	resp, err := c.client.R().
		SetContext(ctx).
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
	return res, nil
}

func (c *Client) GetGroup(ctx context.Context, groupId int64) (*model.Group, error) {
	endpoint := fmt.Sprintf("/groups/%v", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get group %d from %s", groupId, endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.StatusCode() >= 400 {
		return nil, errors.Errorf("failed to get group from %s with status %d: %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}
	rawBody := resp.Body()
	group := &model.Group{}
	if err := json.Unmarshal(rawBody, group); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	group.Init()
	c.CheckBackoff(resp.Header())
	return group, nil
}

func (c *Client) GetGroups(ctx context.Context) ([]*model.Group, error) {
	var endpoint string
	if c.CurrentKey != nil && c.CurrentKey.UserId != 0 {
		endpoint = fmt.Sprintf("/users/%v/groups", c.CurrentKey.UserId)
	} else {
		endpoint = "/users/0/groups"
	}
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get groups from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	if resp.StatusCode() >= 400 {
		return nil, errors.Errorf("failed to get groups from %s with status %d: %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}
	rawBody := resp.Body()
	var groups []*model.Group
	if err := json.Unmarshal(rawBody, &groups); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	for _, g := range groups {
		g.Init()
	}
	c.CheckBackoff(resp.Header())
	return groups, nil
}
