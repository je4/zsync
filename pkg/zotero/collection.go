package zotero

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/xanzy/go-gitlab"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/filesystem"
	"path/filepath"
	"time"
)

type CollectionData struct {
	Key              string       `json:"key"`
	Name             string       `json:"name"`
	Version          int64        `json:"version"`
	Relations        RelationList `json:"relations"`
	ParentCollection Parent       `json:"parentCollection,omitempty"`
}

type CollectionMeta struct {
	NumCollections int64 `json:"numCollections"`
	NumItems       int64 `json:"numItems"`
}

type Collection struct {
	Key     string         `json:"key"`
	Version int64          `json:"version"`
	Library Library        `json:"library,omitempty"`
	Links   interface{}    `json:"links,omitempty"`
	Meta    CollectionMeta `json:"meta,omitempty"`
	Data    CollectionData `json:"data,omitempty"`
	Group   *Group         `json:"-"`
	Status  SyncStatus     `json:"-"`
	Trashed bool           `json:"-"`
	Deleted bool           `json:"-"`
	Gitlab  *time.Time     `json:"-"`
}

type CollectionGitlab struct {
	LibraryId int64          `json:"libraryid"`
	Key       string         `json:"key"`
	Data      CollectionData `json:"data,omitempty"`
	Meta      CollectionMeta `json:"meta,omitempty"`
}

func (collection *Collection) uploadGitlab() (gitlab.EventTypeValue, error) {
	glColl := CollectionGitlab{
		LibraryId: collection.Group.Id,
		Key:       collection.Key,
		Data:      collection.Data,
		Meta:      collection.Meta,
	}

	data, err := json.Marshal(glColl)
	if err != nil {
		return gitlab.ClosedEventType, emperror.Wrapf(err, "cannot marshall data %v", glColl)
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "\t"); err != nil {
		return gitlab.ClosedEventType, emperror.Wrapf(err, "cannot pretty json")
	}
	gcommit := fmt.Sprintf("%v - %v.%v v%v", collection.Data.Name, collection.Group.Id, collection.Key, collection.Version)
	fname := fmt.Sprintf("%v/collections/%v.json", collection.Group.Id, collection.Key)
	var event gitlab.EventTypeValue
	if collection.Deleted || collection.Trashed {
		event, err = collection.Group.Zot.deleteGitlab(fname, "master", gcommit)
		if err != nil {
			return gitlab.ClosedEventType, emperror.Wrapf(err, "update on gitlab failed")
		}
	} else {
		event, err = collection.Group.Zot.uploadGitlab(fname, "master", gcommit, "", prettyJSON.String())
		if err != nil {
			return gitlab.ClosedEventType, emperror.Wrapf(err, "update on gitlab failed")
		}
	}
	return event, nil
}

func (collection *Collection) UpdateLocal() error {
	collection.Group.Zot.logger.Infof("Updating Collection [#%s]", collection.Key)
	data, err := json.Marshal(collection.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", collection.Data)
	}
	meta, err := json.Marshal(collection.Meta)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall meta %v", collection.Meta)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET version=$1, sync=$2, data=$3, meta=$4, deleted=$5, modified=NOW() WHERE key=$6", collection.Group.Zot.dbSchema)
	params := []interface{}{
		collection.Version,
		SyncStatusString[collection.Status],
		data,
		meta,
		collection.Deleted,
		collection.Key,
	}
	_, err = collection.Group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (collection *Collection) UpdateCloud() error {
	collection.Group.Zot.logger.Infof("Creating Zotero Collection [#%s]", collection.Key)

	collection.Data.Version = collection.Version
	if collection.Deleted {
		endpoint := fmt.Sprintf("/groups/%v/collections/%v", collection.Group.Id, collection.Key)
		collection.Group.Zot.logger.Infof("rest call: DELETE %s", endpoint)
		resp, err := collection.Group.Zot.client.R().
			SetHeader("Accept", "application/json").
			SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", collection.Version)).
			Delete(endpoint)
		if err != nil {
			return emperror.Wrapf(err, "create collection %v with %s", collection.Key, endpoint)
		}
		switch resp.RawResponse.StatusCode {
		case 409:
			return errors.New(fmt.Sprintf("delete: Conflict: the target library #%v is locked", collection.Group.Id))
		case 412:
			return errors.New(fmt.Sprintf("delete: Precondition failed: The item #%v.%v has changed since retrieval", collection.Group.Id, collection.Key))
		case 428:
			return errors.New(fmt.Sprintf("delete: Precondition required: If-Unmodified-Since-Version was not provided."))
		}
	} else {
		endpoint := fmt.Sprintf("/groups/%v/collections", collection.Group.Id)
		collection.Group.Zot.logger.Infof("rest call: POST %s", endpoint)
		collections := []CollectionData{collection.Data}
		resp, err := collection.Group.Zot.client.R().
			SetHeader("Accept", "application/json").
			SetBody(collections).
			Post(endpoint)
		if err != nil {
			return emperror.Wrapf(err, "create collection %v with %s", collection.Key, endpoint)
		}
		result := ItemCollectionCreateResult{}
		jsonstr := resp.Body()
		if err := json.Unmarshal(jsonstr, &result); err != nil {
			return emperror.Wrapf(err, "cannot unmarshall result %s", string(jsonstr))
		}
		successKey, err := result.checkSuccess(0)
		if err != nil {
			return emperror.Wrapf(err, "could not create item #%v.%v", collection.Group.Id, collection.Key)
		}
		if successKey != collection.Key {
			return errors.New(fmt.Sprintf("invalid key %s. source key: %s", successKey, collection.Key))
		}
	}
	collection.Status = SyncStatus_Synced
	if err := collection.UpdateLocal(); err != nil {
		return errors.New(fmt.Sprintf("cannot store item in db %v.%v", collection.Group.Id, collection.Key))
	}
	return nil
}

func (collection *Collection) Backup(backupFs filesystem.FileSystem) error {
	collection.Group.Zot.logger.Infof("storing %v to %v", collection.Data.Name, backupFs.String())
	var fname string
	var folder string
	folder = filepath.Clean( fmt.Sprintf("%v/collections", collection.Group.Id))
	fname = filepath.Clean(fmt.Sprintf("%v.json", collection.Key))

	// write data to file
	data := struct {
		LibraryId int64       `json:"libraryid"`
		Id        string      `json:"id"`
		Data      interface{} `json:"data"`
		Meta      interface{} `json:"meta"`
	}{
		LibraryId: collection.Group.Id,
		Id:        collection.Key,
		Data:      collection.Data,
		Meta:      collection.Meta,
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return emperror.Wrapf(err, "cannot marshal data %v", data)
	}
	if err := backupFs.FilePut(folder, fname, b, filesystem.FilePutOptions{}); err != nil {
		return emperror.Wrap(err, "cannot write data to file")
	}

	return nil
}
