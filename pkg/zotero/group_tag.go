package zotero

import (
	"emperror.dev/errors"
	"encoding/json"
	"fmt"
	"strconv"
)

func (group *Group) CreateTagLocal(tag Tag) error {
	group.Zot.Logger.Debug().Msgf("Creating Tag %s", tag.Tag)
	metastr, err := json.Marshal(tag.Meta)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal meta %v", tag.Meta)
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.tags (tag, meta, library) VALUES( $1, $2, $3)", group.Zot.dbSchema)
	params := []interface{}{
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

	params := []interface{}{
		tag,
		group.Id,
	}
	if _, err := group.Zot.db.Exec(sqlstr, params...); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetTagsVersionCloud(sinceVersion int64) (*[]Tag, int64, error) {
	var endpoint string
	endpoint = fmt.Sprintf("/groups/%v/tags", group.Id)
	group.Zot.Logger.Info().Msgf("rest call: %s", endpoint)

	resp, err := group.Zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
		Get(endpoint)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	tags := &[]Tag{}
	if err := json.Unmarshal(rawBody, tags); err != nil {
		return nil, 0, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	limv := resp.RawResponse.Header.Get("Last-Modified-Version")
	lastModifiedVersion, err := strconv.ParseInt(limv, 10, 64)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "cannot convert 'Last-Modified-Version' - %v", limv)
	}

	return tags, lastModifiedVersion, nil
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
	}

	group.Zot.Logger.Info().Msgf("Syncing tags of Group #%v done. %v tags changed", group.Id, counter)
	return counter, lastModifiedVersion, nil
}
