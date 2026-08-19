package zotero

import (
	"emperror.dev/errors"
	"encoding/json"
	"fmt"
	"gopkg.in/resty.v1"
	"strconv"
)

func (group *Group) CreateTagLocal(tag Tag) error {
	group.Zot.Logger.Debug().Msgf("Creating Tag %s", tag.Tag)
	metastr, err := json.Marshal(tag.Meta)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal meta %v", tag.Meta)
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.tags (tag, meta, library) VALUES( $1, $2, $3)", group.Zot.dbSchema)
	params := []any{
		tag.Tag,
		metastr,
		group.Id,
	}
	_, err = group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		if IsUniqueViolation(err, "pk_tags") {
			return nil
		}
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) DeleteTagLocal(tag string) error {
	group.Zot.Logger.Info().Msgf("deleting Tag %s", tag)
	sqlstr := fmt.Sprintf("DELETE FROM %s.tags WHERE tag=$1 and library=$2", group.Zot.dbSchema)

	params := []any{
		tag,
		group.Id,
	}
	if _, err := group.Zot.db.Exec(sqlstr, params...); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetTagsVersionCloud(sinceVersion int64) (*[]Tag, int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/tags", group.Id)

	var lastModifiedVersion int64
	totalTags := []Tag{}
	limit := int64(100)
	start := int64(0)
	for {
		group.Zot.Logger.Info().Msgf("rest call: %s [%v, %v]", endpoint, start, limit)

		call := group.Zot.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))
		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				return nil, 0, errors.Wrapf(err, "cannot get tags from %s", endpoint)
			}
			if !group.Zot.CheckRetry(resp.Header()) {
				break
			}
		}
		rawBody := resp.Body()
		tags := []Tag{}
		if err := json.Unmarshal(rawBody, &tags); err != nil {
			return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		if len(tags) == 0 {
			group.Zot.CheckBackoff(resp.Header())
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
					group.Zot.CheckBackoff(resp.Header())
					break
				}
			} else {
				if int64(len(tags)) < limit {
					group.Zot.CheckBackoff(resp.Header())
					break
				}
			}
		} else {
			if int64(len(tags)) < limit {
				break
			}
		}

		group.Zot.CheckBackoff(resp.Header())
		start += limit
	}

	return &totalTags, lastModifiedVersion, nil
}

func (group *Group) SyncTags() (int64, int64, error) {
	if !group.CanDownload() || !group.syncTags {
		return 0, 0, nil
	}
	group.Zot.Logger.Info().Msgf("Syncing tags of Group #%v", group.Id)
	var counter int64
	tagList, lastModifiedVersion, err := group.GetTagsVersionCloud(group.Version)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get tag versions")
	}
	for _, tag := range *tagList {
		if err := group.CreateTagLocal(tag); err != nil {
			return 0, 0, errors.Wrapf(err, "cannot create tag %v", tag.Tag)
		}
		counter++
	}

	group.Zot.Logger.Info().Msgf("Syncing tags of Group #%v done. %v tags changed", group.Id, counter)
	return counter, lastModifiedVersion, nil
}
