package zotero

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"gopkg.in/resty.v1"
	"os"
	"strconv"
	"strings"
	"time"
)

type GroupMeta struct {
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	NumItems     int64     `json:"numItems"`
}

type GroupData struct {
	Id             int64   `json:"id"`
	Version        int64   `json:"version"`
	Name           string  `json:"name"`
	Owner          int64   `json:owner`
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	Url            string  `json:"url"`
	HasImage       int64   `json:"hasImage"`
	LibraryEditing string  `json:libraryEditing`
	LibraryReading string  `json:libraryReading`
	FileEditing    string  `json:fileEditing`
	Admins         []int64 `json:"admins"`
}

type Group struct {
	Id                int64         `json:"id"`
	Version           int64         `json:"version"`
	Links             interface{}   `json:"links,omitempty"`
	Meta              GroupMeta     `json:"meta"`
	Data              GroupData     `json:"data"`
	Deleted           bool          `json:"-"`
	Active            bool          `json:"-"`
	syncTags          bool          `json:"-"` // sync tags?
	direction         SyncDirection `json:"-"`
	zot               *Zotero       `json:"-"`
	ItemVersion       int64         `json:"-"`
	CollectionVersion int64         `json:"-"`
	TagVersion        int64         `json:"-"`
	Modified          bool          `json:"-"`
	Gitlab            *time.Time    `json:"-"`
}

type GroupGitlab struct {
	Id                int64     `json:"id"`
	Data              GroupData `json:"data,omitempty"`
	CollectionVersion int64     `json:"collectionversion"`
	ItemVersion       int64     `json:"itemversion"`
	TagVersion        int64     `json:tagversion`
}

func (group *Group) uploadGitlab() error {

	glGroup := GroupGitlab{
		Id:                group.Id,
		Data:              group.Data,
		CollectionVersion: group.CollectionVersion,
		ItemVersion:       group.ItemVersion,
		TagVersion:        group.TagVersion,
	}

	data, err := json.Marshal(glGroup)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", glGroup)
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "\t"); err != nil {
		return emperror.Wrapf(err, "cannot pretty json")
	}
	gcommit := fmt.Sprintf("%v - %v v%v", group.Data.Name, group.Id, group.Version)
	fname := fmt.Sprintf("%v.json", group.Id)
	if group.Deleted {
		if err := group.zot.deleteGitlab(fname, "master", gcommit); err != nil {
			return emperror.Wrapf(err, "update on gitlab failed")
		}
	} else {
		if err := group.zot.uploadGitlab(fname, "master", gcommit, "", prettyJSON.String()); err != nil {
			return emperror.Wrapf(err, "update on gitlab failed")
		}
	}
	return nil
}

func (zot *Zotero) DeleteUnknownGroupsLocal(knownGroups []int64) error {
	placeHolder := []string{}
	params := []interface{}{}
	for i := 0; i < len(knownGroups); i++ {
		placeHolder = append(placeHolder, fmt.Sprintf("$%v", i+1))
		params = append(params, sql.NullInt64{
			Int64: knownGroups[i],
			Valid: true,
		})
	}
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET deleted=true WHERE id NOT IN (%s)", zot.dbSchema, strings.Join(placeHolder, ", "))
	_, err := zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, knownGroups)
	}
	sqlstr = fmt.Sprintf("UPDATE %s.syncgroups SET active=false WHERE id NOT IN (%s)", zot.dbSchema, strings.Join(placeHolder, ", "))
	_, err = zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, knownGroups)
	}
	return nil
}

func (zot *Zotero) CreateEmptyGroupLocal(groupId int64) (bool, SyncDirection, error) {
	active := false
	direction := SyncDirection_BothLocal
	sqlstr := fmt.Sprintf("INSERT INTO %s.groups (id,version,created,modified) VALUES($1, 0, NOW(), NOW())", zot.dbSchema)
	_, err := zot.db.Exec(sqlstr, groupId)
	if err != nil {
		return false, SyncDirection_None, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
	}
	sqlstr = fmt.Sprintf("INSERT INTO %s.syncgroups(id,active,direction) VALUES($1, $2, $3)", zot.dbSchema)
	params := []interface{}{
		groupId,
		active,
		SyncDirectionString[direction],
	}
	_, err = zot.db.Exec(sqlstr, params...)
	if err != nil {
		// existiert schon
		if IsUniqueViolation(err, "syncgroups_pkey") {
			var dirstr string
			sqlstr := fmt.Sprintf("SELECT active, direction FROM %s.syncgroups WHERE id=$1", zot.dbSchema)
			if err := zot.db.QueryRow(sqlstr, groupId).Scan(&active, &dirstr); err != nil {
				return false, SyncDirection_None, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
			}
			direction = SyncDirectionId[dirstr]
		} else {
			return false, SyncDirection_None, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
		}
	}
	return active, direction, nil
}

func (group *Group) GetLibrary() *Library {
	return &Library{
		Type:  group.Data.Type,
		Id:    group.Id,
		Name:  group.Data.Name,
		Links: group.Links,
	}
}

func (group *Group) UpdateLocal() error {
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET version=$1, created=$2, modified=$3, data=$4, deleted=$5,"+
		" itemversion=$6, collectionversion=$7, tagversion=$8"+
		" WHERE id=$9", group.zot.dbSchema)
	data, err := json.Marshal(group.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshal Group data")
	}

	params := []interface{}{
		group.Version,
		group.Meta.Created,
		group.Meta.LastModified,
		data,
		group.Deleted,
		group.ItemVersion,
		group.CollectionVersion,
		group.TagVersion,
		group.Id,
	}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, data, "", "\t")
	if err != nil {
		return emperror.Wrapf(err, "cannot pretty json")
	}
	gcommit := fmt.Sprintf("%v - %v v%v", group.Data.Name, group.Id, group.Version)
	var fname string
	fname = fmt.Sprintf("%v.json", group.Id)
	if err := group.zot.uploadGitlab(fname, "master", gcommit, "", prettyJSON.String()); err != nil {
		return emperror.Wrapf(err, "update on gitlab failed")
	}

	return nil
}

func (zot *Zotero) LoadGroupLocal(groupId int64) (*Group, error) {
	group := &Group{
		Id:      groupId,
		Version: 0,
		Links:   nil,
		Meta:    GroupMeta{},
		Data:    GroupData{},
		Active:  zot.newGroupActive,
		zot:     zot,
	}
	zot.logger.Debugf("loading Group #%v from database", groupId)
	sqlstr := fmt.Sprintf("SELECT version, created, modified, data, active, direction, tags,"+
		" itemversion, collectionversion, tagversion"+
		" FROM %s.groups g, %s.syncgroups sg WHERE g.id=sg.id AND g.id=$1", zot.dbSchema, zot.dbSchema)
	row := zot.db.QueryRow(sqlstr, groupId)
	var jsonstr sql.NullString
	var directionstr string
	err := row.Scan(&group.Version,
		&group.Meta.Created,
		&group.Meta.LastModified,
		&jsonstr,
		&group.Active,
		&directionstr,
		&group.syncTags,
		&group.ItemVersion,
		&group.CollectionVersion,
		&group.TagVersion)
	if err != nil {
		// error real error
		if err != sql.ErrNoRows {
			return nil, emperror.Wrapf(err, "error scanning result of %s: %v", sqlstr, groupId)
		}
		// just no data
		active, direction, err := zot.CreateEmptyGroupLocal(groupId)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot create empty Group %v", groupId)
		}
		group.Active = active
		group.direction = direction
	} else {
		group.direction = SyncDirectionId[directionstr]
	}
	if jsonstr.Valid {
		err = json.Unmarshal([]byte(jsonstr.String), &group.Data)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot unmarshall Group data %s", jsonstr)
		}
	}
	return group, nil
}

func (group *Group) GetAttachmentFolder() (string, error) {
	folder := fmt.Sprintf("%s/%v", group.zot.attachmentFolder, group.Id)
	if _, err := os.Stat(folder); err != nil {
		if err := os.Mkdir(folder, 0777); err != nil {
			return "", emperror.Wrapf(err, "cannot create %s", folder)
		}
	}
	return folder, nil
}

func (group *Group) SyncDeleted() (int64, error) {
	if !group.CanDownload() {
		return 0, nil
	}
	endpoint := fmt.Sprintf("/groups/%v/deleted", group.Id)
	group.zot.logger.Infof("rest call: %s [%v]", endpoint, group.Version)
	call := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(group.Version, 10))
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return 0, emperror.Wrapf(err, "cannot get deleted objects from %s", endpoint)
		}
		if !group.zot.CheckRetry(resp.Header()) {
			break
		}
	}
	lastModifiedVersion, err := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64)
	if err != nil {
		return 0, emperror.Wrapf(err, "cannot parse Last-Modified-Version %v", resp.RawResponse.Header.Get("Last-Modified-Version"))
	}
	rawBody := resp.Body()
	deletions := Deletions{}
	if err := json.Unmarshal(rawBody, &deletions); err != nil {
		return 0, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	for _, itemKey := range deletions.Items {
		if err := group.TryDeleteItemLocal(itemKey, lastModifiedVersion); err != nil {
			return 0, emperror.Wrapf(err, "cannot delete item %v", itemKey)
		}
	}
	for _, collectionKey := range deletions.Collections {
		if err := group.TryDeleteCollectionLocal(collectionKey, lastModifiedVersion); err != nil {
			return 0, emperror.Wrapf(err, "cannot delete collection %v", collectionKey)
		}
	}
	for _, tagName := range deletions.Tags {
		if err := group.DeleteTagLocal(tagName); err != nil {
			return 0, emperror.Wrapf(err, "cannot delete tag %v", tagName)
		}
	}

	group.zot.CheckBackoff(resp.Header())

	return int64(len(deletions.Tags) + len(deletions.Items) + len(deletions.Collections)), nil
}

func (group *Group) Sync() (err error) {
	// no sync at all
	if group.direction == SyncDirection_None {
		return nil
	}

	_, collectionVersion, err := group.SyncCollections()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync collections of Group %v", group.Id)
	}
	_, itemVersion, err := group.SyncItems()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}
	_, tagVersion, err := group.SyncTags()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync tags of Group %v", group.Id)
	}

	_, err = group.SyncDeleted()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync tags of Group %v", group.Id)
	}

	// change to new version if everything was ok
	if collectionVersion > group.CollectionVersion {
		group.CollectionVersion = collectionVersion
		group.Modified = true
	}
	if itemVersion > group.ItemVersion {
		group.ItemVersion = itemVersion
		group.Modified = true
	}
	if tagVersion > group.TagVersion {
		group.TagVersion = tagVersion
		group.Modified = true
	}

	return
}

func (group *Group) CanUpload() bool {
	return group.direction == SyncDirection_BothCloud ||
		group.direction == SyncDirection_BothLocal ||
		group.direction == SyncDirection_BothManual ||
		group.direction == SyncDirection_ToCloud
}

func (group *Group) CanDownload() bool {
	return group.direction == SyncDirection_BothCloud ||
		group.direction == SyncDirection_BothLocal ||
		group.direction == SyncDirection_BothManual ||
		group.direction == SyncDirection_ToLocal
}
