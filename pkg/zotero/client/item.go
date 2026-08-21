package client

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/info"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

func (c *Client) GetItemsVersion(groupId int64, sinceVersion int64, trashed bool) (*map[string]int64, int64, error) {
	var endpoint string
	if trashed {
		endpoint = fmt.Sprintf("/groups/%v/items/trash", groupId)
	} else {
		endpoint = fmt.Sprintf("/groups/%v/items", groupId)
	}

	totalObjects := &map[string]int64{}
	limit := int64(500)
	start := int64(0)
	var lastModifiedVersion int64
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
				return nil, 0, errors.Wrapf(err, "cannot get item versions from %s", endpoint)
			}
			if !c.CheckRetry(resp.Header()) {
				break
			}
		}
		rawBody := resp.Body()
		objects := &map[string]int64{}
		if err := json.Unmarshal(rawBody, objects); err != nil {
			return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		totalResult, err := strconv.ParseInt(resp.RawResponse.Header.Get("Total-Results"), 10, 64)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "cannot parse Total-Results %v", resp.RawResponse.Header.Get("Total-Results"))
		}
		limv := resp.RawResponse.Header.Get("Last-Modified-Version")
		h, err := strconv.ParseInt(limv, 10, 64)
		if err == nil && h > lastModifiedVersion {
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

func (c *Client) GetItemsByKey(groupId int64, objectKeys []string) (*[]model.Item, error) {
	if len(objectKeys) == 0 {
		return &[]model.Item{}, nil
	}
	if len(objectKeys) > 50 {
		return nil, errors.New("too many objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/items", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("itemKey", strings.Join(objectKeys, ",")).
		SetQueryParam("includeTrashed", "1")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get items from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	if c.Logger != nil {
		c.Logger.Debug().Msgf("status: #%v ", resp.StatusCode())
	}
	rawBody := resp.Body()
	items := []model.Item{}
	if err := json.Unmarshal(rawBody, &items); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	c.CheckBackoff(resp.Header())
	result := []model.Item{}
	for _, item := range items {
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
		result = append(result, item)
	}
	return &result, nil
}

func (c *Client) GetItemsTrashByKey(groupId int64, objectKeys []string) (*[]model.Item, error) {
	if len(objectKeys) == 0 {
		return &[]model.Item{}, nil
	}
	if len(objectKeys) > 50 {
		return nil, errors.New("too many objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/items/trash", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	call := c.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("itemKey", strings.Join(objectKeys, ","))
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get items from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	if c.Logger != nil {
		c.Logger.Debug().Msgf("status: #%v ", resp.StatusCode())
	}
	if resp.StatusCode() == 404 {
		return &[]model.Item{}, nil
	}
	if resp.StatusCode() >= 400 {
		return nil, errors.Errorf("failed to get items from %s with status %d: %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}
	rawBody := resp.Body()
	items := []model.Item{}
	if err := json.Unmarshal(rawBody, &items); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	c.CheckBackoff(resp.Header())
	result := []model.Item{}
	for _, item := range items {
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
		result = append(result, item)
	}
	return &result, nil
}

func (c *Client) GetItemByKey(groupId int64, key string) (*model.Item, error) {
	endpoint := fmt.Sprintf("/groups/%v/items/%v", groupId, key)
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
			return nil, errors.Wrapf(err, "cannot get item %s from %s", key, endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	rawBody := resp.Body()
	item := &model.Item{}
	if err := json.Unmarshal(rawBody, item); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	c.CheckBackoff(resp.Header())
	if item.Data.Collections == nil {
		item.Data.Collections = []string{}
	}
	return item, nil
}

func (c *Client) GetItemsQuery(groupId int64, queryParams map[string]string) (*[]model.Item, *resty.Response, error) {
	endpoint := fmt.Sprintf("/groups/%v/items", groupId)
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
			return nil, nil, errors.Wrapf(err, "cannot get items from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	rawBody := resp.Body()
	items := []model.Item{}
	if err := json.Unmarshal(rawBody, &items); err != nil {
		return nil, resp, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	result := []model.Item{}
	for _, item := range items {
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
		result = append(result, item)
	}
	return &result, resp, nil
}

func (c *Client) CreateItems(groupId int64, items []model.ItemGeneric, lastModifiedVersion *int64) (*model.ItemCollectionCreateResult, error) {
	endpoint := fmt.Sprintf("/groups/%v/items", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}
	req := c.client.R().
		SetHeader("Accept", "application/json").
		SetBody(items)
	if lastModifiedVersion != nil && *lastModifiedVersion > 0 {
		req.SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", *lastModifiedVersion))
	}

	var resp *resty.Response
	var err error
	for {
		resp, err = req.Post(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "create items with %s", endpoint)
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
		return nil, errors.Errorf("create items failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	result := &model.ItemCollectionCreateResult{}
	if err := json.Unmarshal(resp.Body(), result); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal result %s", string(resp.Body()))
	}
	if lastModifiedVersion != nil {
		if h, err := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64); err == nil && h > *lastModifiedVersion {
			*lastModifiedVersion = h
		}
	}
	return result, nil
}

func (c *Client) UpdateItem(groupId int64, item *model.ItemGeneric, lastModifiedVersion *int64) (string, error) {
	endpoint := fmt.Sprintf("/groups/%v/items", groupId)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}
	sendData := *item
	if sendData.Version == 0 {
		sendData.Key = ""
	}
	items := []model.ItemGeneric{sendData}
	req := c.client.R().
		SetHeader("Accept", "application/json").
		SetBody(items)
	if lastModifiedVersion != nil && *lastModifiedVersion > 0 {
		req.SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", *lastModifiedVersion))
	}

	var resp *resty.Response
	var err error
	for {
		resp, err = req.Post(endpoint)
		if err != nil {
			return "", errors.Wrapf(err, "update item %v with %s", item.Key, endpoint)
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
		return "", errors.Errorf("update item failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
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
		return "", errors.Wrapf(err, "could not update item #%v.%v", groupId, item.Key)
	}
	return successKey, nil
}

func (c *Client) DeleteItem(groupId int64, itemKey string, lastModifiedVersion int64) error {
	endpoint := fmt.Sprintf("/groups/%v/items/%v", groupId, itemKey)
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
			return errors.Wrapf(err, "delete item %v with %s", itemKey, endpoint)
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
		return errors.Errorf("delete: Precondition failed: The item #%v.%v has changed since retrieval", groupId, itemKey)
	case 428:
		return errors.New("delete: Precondition required: If-Unmodified-Since-Version was not provided.")
	default:
		return errors.Errorf("delete item failed with status code %d", resp.StatusCode())
	}
}

func (c *Client) DownloadAttachment(groupId int64, itemKey string) ([]byte, string, string, error) {
	endpoint := fmt.Sprintf("/groups/%v/items/%s/file", groupId, itemKey)
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
			return nil, "", "", errors.Wrapf(err, "cannot get attachment from %s", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	body := resp.Body()
	contentType := resp.Header().Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	md5str := resp.Header().Get("ETag")
	md5str = strings.Trim(md5str, "\"")
	if md5str == "" {
		md5sink := md5.New()
		md5str = fmt.Sprintf("%x", md5sink.Sum(body))
	}
	c.CheckBackoff(resp.Header())
	return body, contentType, md5str, nil
}

func (c *Client) UploadAttachment(groupId int64, itemKey string, data []byte, filename string, contentType string, mtime int64, md5Hash string) (string, error) {
	if md5Hash == "" {
		md5sink := md5.New()
		md5Hash = fmt.Sprintf("%x", md5sink.Sum(data))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	endpoint := fmt.Sprintf("/groups/%v/items/%v/file", groupId, itemKey)
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}

	authAttempts := 0
	var resp *resty.Response
	var err error
	for {
		authAttempts++
		resp, err = c.client.R().
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			SetHeader("If-None-Match", "*").
			SetFormData(map[string]string{
				"md5":      md5Hash,
				"filename": filename,
				"filesize": fmt.Sprintf("%v", len(data)),
				"mtime":    fmt.Sprintf("%v", mtime),
			}).
			Post(endpoint)
		if err != nil {
			return "", errors.Wrapf(err, "upload attachment auth for item %v with %s", itemKey, endpoint)
		}
		if resp.StatusCode() == 429 && authAttempts < 5 {
			if c.CheckRetry(resp.Header()) {
				continue
			}
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	switch resp.StatusCode() {
	case 200:
	case 403:
		return "", errors.Errorf("file editing denied for item %v with %s", itemKey, endpoint)
	case 412:
		return "", errors.Errorf("file precondition failed. please solve conflict for item %v with %s", itemKey, endpoint)
	case 413:
		return "", errors.Errorf("file too large. please upgrade storage for item %v with %s", itemKey, endpoint)
	case 428:
		return "", errors.Errorf("file precondition required. If-Match or If-None-Match was not provided for item %v with %s", itemKey, endpoint)
	case 429:
		return "", errors.Errorf("file too many requests. Too many unfinished uploads for item %v with %s", itemKey, endpoint)
	default:
		return "", errors.Errorf("file unknown error #%v for item %v with %s", resp.Status(), itemKey, endpoint)
	}

	var result map[string]string
	if err = json.Unmarshal(resp.Body(), &result); err != nil {
		return "", errors.Wrapf(err, "cannot unmarshal result %s", string(resp.Body()))
	}
	if _, ok := result["exists"]; ok {
		// already there
		return md5Hash, nil
	}

	/**
	Upload file (Amazon S3 e.a.)
	*/
	uploadUrl, ok := result["url"]
	if !ok {
		return "", errors.Errorf("no url in upload authorization %v", string(resp.Body()))
	}
	resultContentType, ok := result["contentType"]
	if !ok {
		resultContentType = contentType
	}
	prefix, ok := result["prefix"]
	if !ok {
		prefix = ""
	}
	suffix, ok := result["suffix"]
	if !ok {
		suffix = ""
	}
	uploadKey, ok := result["uploadKey"]
	if !ok {
		return "", errors.Errorf("no uploadKey in upload authorization %v", string(resp.Body()))
	}

	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", uploadUrl)
	}
	uploadResp, err := resty.New().R().
		SetHeader("User-Agent", info.GetUserAgent()).
		SetHeader("Content-Type", resultContentType).
		SetBody(append([]byte(prefix), append(data, []byte(suffix)...)...)).
		Post(uploadUrl)
	if err != nil {
		return "", errors.Wrapf(err, "error uploading file to %v", uploadUrl)
	}
	if uploadResp.StatusCode() != 201 && uploadResp.StatusCode() != 200 && uploadResp.StatusCode() != 204 {
		return "", errors.Errorf("error uploading file with status %v - %v", uploadResp.Status(), uploadResp.Body())
	}

	/**
	register upload
	*/
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: POST %s", endpoint)
	}
	h := c.client.R().
		SetHeader("If-None-Match", "*")
	for {
		resp, err = h.
			SetFormData(map[string]string{"upload": uploadKey}).
			Post(endpoint)
		if err != nil {
			return "", errors.Wrapf(err, "cannot register upload %v", endpoint)
		}
		if !c.CheckRetry(resp.Header()) {
			break
		}
	}
	c.CheckBackoff(resp.Header())
	switch resp.StatusCode() {
	case 200, 204:
	case 412:
		return "", errors.Errorf("Precondition failed - The file has changed remotely since retrieval for item %v.%v", groupId, itemKey)
	default:
		return "", errors.Errorf("register upload failed with status %v", resp.StatusCode())
	}
	return md5Hash, nil
}
