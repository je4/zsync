package client

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

func (c *Client) GetCollectionVersions(groupId int64, sinceVersion int64) (*map[string]int64, int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections", groupId)

	totalObjects := &map[string]int64{}
	var lastModifiedVersion int64
	limit := int64(500)
	start := int64(0)
	for {
		if c.Logger != nil {
			c.Logger.Info().Msgf("rest call: %s [%v, %v]", endpoint, start, limit)
		}

		call := c.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
			SetQueryParam("format", "versions").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))
		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				return nil, 0, errors.Wrapf(err, "cannot get collection versions from %s", endpoint)
			}
			if !c.CheckRetry(resp.Header()) {
				break
			}
		}
		totalResult, err := strconv.ParseInt(resp.RawResponse.Header.Get("Total-Results"), 10, 64)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "cannot parse Total-Results %v", resp.RawResponse.Header.Get("Total-Results"))
		}
		rawBody := resp.Body()
		objects := &map[string]int64{}
		if err := json.Unmarshal(rawBody, objects); err != nil {
			return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		limv := resp.RawResponse.Header.Get("Last-Modified-Version")
		h, err := strconv.ParseInt(limv, 10, 64)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "cannot convert 'Last-Modified-Version' - %v", limv)
		}
		if h > lastModifiedVersion {
			lastModifiedVersion = h
		}

		c.CheckBackoff(resp.Header())
		maps.Copy((*totalObjects), *objects)
		if totalResult <= start+limit {
			break
		}
		start += limit
	}

	return totalObjects, lastModifiedVersion, nil
}

func (c *Client) GetCollectionsByKey(groupId int64, objectKeys []string) (*[]model.Collection, int64, error) {
	if len(objectKeys) == 0 {
		return &[]model.Collection{}, 0, errors.New("no objectKeys")
	}
	if len(objectKeys) > 50 {
		return nil, 0, errors.New("too many objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/collections", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("collectionKey", strings.Join(objectKeys, ","))

	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "cannot get collections from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	rawBody := resp.Body()
	collections := []model.Collection{}
	if err := json.Unmarshal(rawBody, &collections); err != nil {
		return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	limv := resp.RawResponse.Header.Get("Last-Modified-Version")
	lastModifiedVersion, err := strconv.ParseInt(limv, 10, 64)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "cannot convert 'Last-Modified-Version' - %v", limv)
	}

	c.CheckBackoff(resp.Header())
	result := []model.Collection{}
	for _, coll := range collections {
		if strings.ToLower(coll.Library.Type) != "group" {
			return nil, 0, errors.Errorf("unknown library type %v for collection %v", coll.Library.Type, coll.Key)
		}
		if coll.Library.Id != groupId {
			return nil, 0, errors.Errorf("wrong library id %v for collection %v - current Group is %v", coll.Library.Id, coll.Key, groupId)
		}
		result = append(result, coll)
	}
	return &result, lastModifiedVersion, nil
}

func (c *Client) GetCollectionByKey(groupId int64, key string) (*model.Collection, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections/%v", groupId, key)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetHeader("Accept", "application/json")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get collection %s from %s", key, endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	rawBody := resp.Body()
	coll := &model.Collection{}
	if err := json.Unmarshal(rawBody, coll); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	c.CheckBackoff(resp.Header())
	return coll, nil
}

func (c *Client) GetCollectionsQuery(groupId int64, queryParams map[string]string) (*[]model.Collection, *resty.Response, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParams(queryParams)
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "cannot get collections from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	rawBody := resp.Body()
	collections := []model.Collection{}
	if err := json.Unmarshal(rawBody, &collections); err != nil {
		return nil, resp, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return &collections, resp, nil
}

func (c *Client) CreateCollections(groupId int64, collections []model.CollectionData) (*model.ItemCollectionCreateResult, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}

	req := c.client.R().
		SetHeader("Accept", "application/json").
		SetBody(collections)

	var resp *resty.Response
	var err error
	for {
		resp, err = req.Post(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot post collections to %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	if resp.StatusCode() >= 400 {
		return nil, errors.Errorf("create collections failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	result := &model.ItemCollectionCreateResult{}
	if err := json.Unmarshal(resp.Body(), result); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal create collections result: %s", string(resp.Body()))
	}
	return result, nil
}

func (c *Client) UpdateCollection(groupId int64, collection *model.CollectionData, lastModifiedVersion *int64) (string, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}
	sendData := *collection
	if sendData.Version == 0 {
		sendData.Key = ""
	}
	collections := []model.CollectionData{sendData}
	req := c.client.R().
		SetHeader("Accept", "application/json").
		SetBody(collections)
	if lastModifiedVersion != nil && *lastModifiedVersion > 0 {
		req.SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", *lastModifiedVersion))
	}

	var resp *resty.Response
	var err error
	for {
		resp, err = req.Post(endpoint)
		if err != nil {
			return "", errors.Wrapf(err, "update collection %v with %s", collection.Key, endpoint)
		}
		if resp.StatusCode() == 412 && lastModifiedVersion != nil {
			if lmv, lErr := strconv.ParseInt(resp.Header().Get("Last-Modified-Version"), 10, 64); lErr == nil && lmv > 0 {
				*lastModifiedVersion = lmv
				req.SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", *lastModifiedVersion))
				continue
			}
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	if resp.StatusCode() >= 400 {
		return "", errors.Errorf("update collection failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	result := &model.ItemCollectionCreateResult{}
	if err := json.Unmarshal(resp.Body(), result); err != nil {
		return "", errors.Wrapf(err, "cannot unmarshal result %s", string(resp.Body()))
	}
	if lastModifiedVersion != nil {
		if h, err := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64); err == nil && h > *lastModifiedVersion {
			*lastModifiedVersion = h
		}
	}
	successKey, err := result.CheckSuccess(0)
	if err != nil {
		return "", errors.Wrapf(err, "could not update collection #%v.%v", groupId, collection.Key)
	}
	return successKey, nil
}

func (c *Client) DeleteCollection(groupId int64, collectionKey string, lastModifiedVersion int64) error {
	endpoint := fmt.Sprintf("/groups/%v/collections/%v", groupId, collectionKey)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: DELETE %s", endpoint)
	}
	req := c.client.R().
		SetHeader("Accept", "application/json")
	if lastModifiedVersion > 0 {
		req.SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", lastModifiedVersion))
	}

	var resp *resty.Response
	var err error
	for {
		resp, err = req.Delete(endpoint)
		if err != nil {
			return errors.Wrapf(err, "delete collection %v with %s", collectionKey, endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	switch resp.StatusCode() {
	case 200, 204:
		return nil
	case 409:
		return errors.Errorf("delete: Conflict: the target library #%v is locked", groupId)
	case 412:
		return errors.Errorf("delete: Precondition failed: The item #%v.%v has changed since retrieval", groupId, collectionKey)
	case 428:
		return errors.New("delete: Precondition required: If-Unmodified-Since-Version was not provided.")
	default:
		return errors.Errorf("delete collection failed with status code %d", resp.StatusCode())
	}
}
