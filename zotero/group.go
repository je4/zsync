package zotero

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"github.com/lib/pq"
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
	Id      int64       `json:"id"`
	Version int64       `json:"version"`
	Links   interface{} `json:"links,omitempty"`
	Meta    GroupMeta   `json:"meta"`
	Data    GroupData   `json:"data"`
	Deleted bool        `json:"-"`
	Active  bool        `json:"-"`
	zot     *Zotero     `json:"-"`
}

func (zot *Zotero) DeleteUnknownGroups(knownGroups []int64) error {
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

func (zot *Zotero) CreateEmptyGroupDB(groupId int64) (bool, error) {
	active := true
	sqlstr := fmt.Sprintf("INSERT INTO %s.groups (id,version,created,lastmodified) VALUES($1, 0, NOW(), NOW())", zot.dbSchema)
	_, err := zot.db.Exec(sqlstr, groupId)
	if err != nil {
		return false, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
	}
	sqlstr = fmt.Sprintf("INSERT INTO %s.syncgroups(id,active) VALUES($1, false)", zot.dbSchema)
	_, err = zot.db.Exec(sqlstr, groupId)
	if pqError, ok := err.(*pq.Error); ok {
		switch {
		/*
			Class 23 — Integrity Constraint Violation
			23000	INTEGRITY CONSTRAINT VIOLATION	integrity_constraint_violation
			23001	RESTRICT VIOLATION	restrict_violation
			23502	NOT NULL VIOLATION	not_null_violation
			23503	FOREIGN KEY VIOLATION	foreign_key_violation
			23505	UNIQUE VIOLATION	unique_violation
			23514	CHECK VIOLATION	check_violation
		*/
		case pqError.Code == "23505":
			sqlstr := fmt.Sprintf("SELECT active FROM %s.syncgroups WHERE id=$1", zot.dbSchema)
			if err := zot.db.QueryRow(sqlstr, groupId).Scan(&active); err != nil {
				return false, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
			}
		default:
			return false, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
		}
	}
	return active, nil
}

func (group *Group) GetLibrary() *Library {
	return &Library{
		Type:  group.Data.Type,
		Id:    group.Id,
		Name:  group.Data.Name,
		Links: group.Links,
	}
}

func (group *Group) UpdateDB() error {
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET version=$1, created=$2, lastmodified=$3, data=$4, deleted=$5 WHERE id=$6", group.zot.dbSchema)
	data, err := json.Marshal(group.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshal group data")
	}

	params := []interface{}{group.Version, group.Meta.Created, group.Meta.LastModified, data, group.Deleted, group.Id}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return nil
}

func (zot *Zotero) LoadGroupDB(groupId int64) (*Group, error) {
	group := &Group{
		Id:      groupId,
		Version: 0,
		Links:   nil,
		Meta:    GroupMeta{},
		Data:    GroupData{},
		Active:  zot.newGroupActive,
		zot:     zot,
	}
	zot.logger.Debugf("loading group #%v from database", groupId)
	sqlstr := fmt.Sprintf("SELECT version, created, lastmodified, data, active FROM %s.groups g, %s.syncgroups sg WHERE g.id=sg.id AND g.id=$1", zot.dbSchema, zot.dbSchema)
	row := zot.db.QueryRow(sqlstr, groupId)
	var jsonstr sql.NullString
	err := row.Scan(&group.Version, &group.Meta.Created, &group.Meta.LastModified, &jsonstr, &group.Active)
	if err != nil && err != sql.ErrNoRows {
		return nil, emperror.Wrapf(err, "error scanning result of %s: %v", sqlstr, groupId)
	}
	if err == sql.ErrNoRows {
		active, err := zot.CreateEmptyGroupDB(groupId)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot create empty group %v", groupId)
		}
		group.Active = active
	}
	if jsonstr.Valid {
		err = json.Unmarshal([]byte(jsonstr.String), &group.Data)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot unmarshall group data %s", jsonstr)
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

func (group *Group) SyncDeleted() error {
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
			return emperror.Wrapf(err, "cannot get deleted objects from %s", endpoint)
		}
		if !group.zot.CheckRetry(resp.Header()) {
			break
		}
	}
	lastModifiedVersion, err := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64)
	if err != nil {
		return emperror.Wrapf(err, "cannot parse Last-Modified-Version %v", resp.RawResponse.Header.Get("Last-Modified-Version"))
	}
	rawBody := resp.Body()
	deletions := Deletions{}
	if err := json.Unmarshal(rawBody, deletions); err != nil {
		return emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}


	group.zot.CheckBackoff(resp.Header())

	return nil
}
