package client

import (
	"context"
	"encoding/json/v2"
	"maps"
	"strconv"

	"emperror.dev/errors"
	"gopkg.in/resty.v1"
)

// fetchPaginatedSlices retrieves paginated JSON items as a slice of type T.
func fetchPaginatedSlices[T any](
	ctx context.Context,
	c *Client,
	endpoint string,
	limit int64,
	setupParams func(req *resty.Request),
) ([]T, int64, error) {
	var totalItems []T
	var lastModifiedVersion int64
	var start int64 = 0

	for {
		if c.Logger != nil {
			c.Logger.Info().Msgf("rest call: %s [%v, %v]", endpoint, start, limit)
		}

		call := c.client.R().
			SetContext(ctx).
			SetHeader("Accept", "application/json").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))

		if setupParams != nil {
			setupParams(call)
		}

		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				return nil, 0, errors.Wrapf(err, "cannot get data from %s", endpoint)
			}
			if !c.CheckRetry(resp.Header()) {
				break
			}
		}

		rawBody := resp.Body()
		var pageItems []T
		if err := json.Unmarshal(rawBody, &pageItems); err != nil {
			return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}

		if len(pageItems) == 0 {
			c.CheckBackoff(resp.Header())
			break
		}

		totalItems = append(totalItems, pageItems...)

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
				if err == nil && int64(len(totalItems)) >= totalResult {
					c.CheckBackoff(resp.Header())
					break
				}
			} else if int64(len(pageItems)) < limit {
				c.CheckBackoff(resp.Header())
				break
			}
		} else if int64(len(pageItems)) < limit {
			c.CheckBackoff(resp.Header())
			break
		}

		c.CheckBackoff(resp.Header())
		start += limit
	}

	return totalItems, lastModifiedVersion, nil
}

// fetchPaginatedMap retrieves paginated JSON items as a map[K]V.
func fetchPaginatedMap[K comparable, V any](
	ctx context.Context,
	c *Client,
	endpoint string,
	limit int64,
	setupParams func(req *resty.Request),
) (map[K]V, int64, error) {
	totalObjects := make(map[K]V)
	var lastModifiedVersion int64
	var start int64 = 0

	for {
		if c.Logger != nil {
			c.Logger.Info().Msgf("rest call: %s [%v, %v]", endpoint, start, limit)
		}

		call := c.client.R().
			SetContext(ctx).
			SetHeader("Accept", "application/json").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))

		if setupParams != nil {
			setupParams(call)
		}

		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				return nil, 0, errors.Wrapf(err, "cannot get data from %s", endpoint)
			}
			if !c.CheckRetry(resp.Header()) {
				break
			}
		}

		rawBody := resp.Body()
		pageObjects := make(map[K]V)
		if err := json.Unmarshal(rawBody, &pageObjects); err != nil {
			return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}

		maps.Copy(totalObjects, pageObjects)

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
				if err == nil {
					if int64(len(totalObjects)) >= totalResult || totalResult <= start+limit {
						c.CheckBackoff(resp.Header())
						break
					}
				} else if int64(len(pageObjects)) < limit {
					c.CheckBackoff(resp.Header())
					break
				}
			} else if int64(len(pageObjects)) < limit {
				c.CheckBackoff(resp.Header())
				break
			}
		} else if int64(len(pageObjects)) < limit {
			c.CheckBackoff(resp.Header())
			break
		}

		c.CheckBackoff(resp.Header())
		start += limit
	}

	return totalObjects, lastModifiedVersion, nil
}
