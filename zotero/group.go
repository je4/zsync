package zotero

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"os"
	"strings"
	"time"
)

type GroupMeta struct {
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	NumItems     int64     `json:"numItems"`
}

type Group struct {
	Id      int64       `json:"id"`
	Version int64       `json:"version"`
	Links   interface{} `json:"links,omitempty"`
	Meta    GroupMeta   `json:"meta"`
	Data    interface{} `json:"data"`
	Deleted bool        `json:"-"`
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
	return nil
}

func (zot *Zotero) CreateEmptyGroupDB(groupId int64) error {
	sqlstr := fmt.Sprintf("INSERT INTO %s.groups (id,version,created,lastmodified) VALUES($1, 0, NOW(), NOW())", zot.dbSchema)
	_, err := zot.db.Exec(sqlstr, groupId)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
	}
	return nil
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
		Data:    nil,
		zot:     zot,
	}
	sqlstr := fmt.Sprintf("SELECT version, created, lastmodified, data FROM %s.groups WHERE id=$1", zot.dbSchema)
	row := zot.db.QueryRow(sqlstr, groupId)
	if err := row.Scan(&group.Version, &group.Meta.Created, &group.Meta.LastModified, &group.Data); err != nil && err != sql.ErrNoRows {
		return nil, emperror.Wrapf(err, "error scanning result of %s: %v", sqlstr, groupId)
	}
	return group, nil
}

func (group *Group) GetAttachmentFolder() (string,error) {
	folder := fmt.Sprintf("%s/%v", group.zot.attachmentFolder, group.Id)
	if _, err := os.Stat(folder); err != nil {
		if err := os.Mkdir(folder, 0777); err != nil {
			return "", emperror.Wrapf(err, "cannot create %s", folder)
		}
	}
	return folder, nil
}