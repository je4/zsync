package client

import (
	"encoding/json"
	"fmt"
	"strconv"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

func (c *Client) GetTagsVersion(groupId int64, sinceVersion int64) (*[]model.Tag, int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/tags", groupId)

	var lastModifiedVersion int64
	totalTags := []model.Tag{}
	limit := int64(100)
	start := int64(0)
	for {
		if c.Logger != nil {
			c.Logger.Info().Msgf("rest call: %s [%v, %v]", endpoint, start, limit)
		}

		call := c.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))
		if sinceVersion > 0 {
			call.SetQueryParam("since", strconv.FormatInt(sinceVersion, 10))
		}
		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				return nil, 0, errors.Wrapf(err, "cannot get tags from %s", endpoint)
			}
			if !c.CheckRetry(resp.Header()) {
				break
			}
		}
		rawBody := resp.Body()
		tags := []model.Tag{}
		if err := json.Unmarshal(rawBody, &tags); err != nil {
			return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		if len(tags) == 0 {
			c.CheckBackoff(resp.Header())
			break
		}
		totalTags = append(totalTags, tags...)

		if resp.RawResponse != nil && resp.RawResponse.Header != nil {
			limv := resp.RawResponse.Header.Get("Last-Modified-Version")
			if limv != "" {
				h, err := strconv.ParseInt(limv, 10, 64)
				if err == nil && h > lastModifiedVersion {
					lastModifiedVersion = h
				}
			}

			totalResultStr := resp.RawResponse.Header.Get("Total-Results")
			if totalResultStr != "" {
				totalResult, err := strconv.ParseInt(totalResultStr, 10, 64)
				if err == nil && int64(len(totalTags)) >= totalResult {
					c.CheckBackoff(resp.Header())
					break
				}
			} else {
				if int64(len(tags)) < limit {
					c.CheckBackoff(resp.Header())
					break
				}
			}
		} else {
			if int64(len(tags)) < limit {
				c.CheckBackoff(resp.Header())
				break
			}
		}

		c.CheckBackoff(resp.Header())
		start += limit
	}

	return &totalTags, lastModifiedVersion, nil
}
