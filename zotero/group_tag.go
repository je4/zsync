package zotero

import (
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"strconv"
)

func (group *Group) CreateTagLocal(tag Tag) error {
	group.zot.logger.Debugf("Creating Tag %s", tag.Tag)
	metastr, err := json.Marshal(tag.Meta)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshal meta %v", tag.Meta)
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.tags (tag, meta, library) VALUES( $1, $2, $3)", group.zot.dbSchema)
	params := []interface{}{
		tag.Tag,
		metastr,
		group.Id,
	}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		if IsUniqueViolation(err, "pk_tags") {
			return nil
		}
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) DeleteTagLocal(tag string) error {
	group.zot.logger.Infof("deleting Tag %s", tag)
	sqlstr := fmt.Sprintf("DELETE FROM %s.tags WHERE tag=$1 and library=$2", group.zot.dbSchema)

	params := []interface{}{
		tag,
		group.Id,
	}
	if _, err := group.zot.db.Exec(sqlstr, params...); err != nil {
		return emperror.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetTagsVersionCloud(sinceVersion int64) (*[]Tag, int64, error) {
	var endpoint string
	endpoint = fmt.Sprintf("/groups/%v/tags", group.Id)
	group.zot.logger.Infof("rest call: %s", endpoint)

	resp, err := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
		Get(endpoint)
	if err != nil {
		return nil, 0, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	tags := &[]Tag{}
	if err := json.Unmarshal(rawBody, tags); err != nil {
		return nil, 0, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	lastModifiedVersion, _ := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64)

	return tags, lastModifiedVersion, nil
}

func (group *Group) SyncTags() (int64, int64, error) {
	if !group.CanDownload() || !group.syncTags {
		return 0, 0, nil
	}
	group.zot.logger.Infof("Syncing tags of group #%v", group.Id)
	var counter int64
	tagList, lastModifiedVersion, err := group.GetTagsVersionCloud(group.Version)
	if err != nil {
		return counter, 0, emperror.Wrapf(err, "cannot get tag versions")
	}
	for _, tag := range *tagList {
		if err := group.CreateTagLocal(tag); err != nil {
			return 0, 0, emperror.Wrapf(err, "cannot create tag %v", tag.Tag)
		}
	}

	group.zot.logger.Infof("Syncing tags of group #%v done. %v tags changed", group.Id, counter)
	return counter, lastModifiedVersion, nil
}
