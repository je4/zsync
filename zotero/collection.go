package zotero

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"time"
)

type CollectionData struct {
	Key              string            `json:"key"`
	Name             string            `json:"name"`
	Version          int64             `json:"version,omitempty"`
	Relations        map[string]string `json:"relations"`
	ParentCollection Parent            `json:"parentCollection,omitempty"`
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
	group   *Group         `json:"-"`
	Status  SyncStatus     `json:"-"`
	Trashed bool           `json:"-"`
	Deleted bool           `json:"-"`
	Gitlab  *time.Time     `json:"-"`
}

type CollectionGitlab struct {
	LibraryId int64          `json:"libraryid"`
	Key       string         `json:"key"`
	Meta      CollectionMeta `json:"meta,omitempty"`
	Data      CollectionData `json:"data,omitempty"`
}

func (collection *Collection) uploadGitlab() error {

	glColl := CollectionGitlab{
		LibraryId: collection.group.Id,
		Key:       collection.Key,
		Data:      collection.Data,
		Meta:      collection.Meta,
	}

	data, err := json.Marshal(glColl)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", glColl)
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "\t"); err != nil {
		return emperror.Wrapf(err, "cannot pretty json")
	}
	gcommit := fmt.Sprintf("%v - %v.%v v%v", collection.Data.Name, collection.group.Id, collection.Key, collection.Version)
	fname := fmt.Sprintf("%v/collections/%v.json", collection.group.Id, collection.Key)
	if err := collection.group.zot.uploadGitlab(fname, "master", gcommit, "", prettyJSON.String()); err != nil {
		return emperror.Wrapf(err, "update on gitlab failed")
	}
	return nil
}

func (collection *Collection) UpdateLocal() error {
	collection.group.zot.logger.Infof("Updating Collection [#%s]", collection.Key)
	data, err := json.Marshal(collection.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", collection.Data)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET version=$1, sync=$2, data=$3, deleted=$4, modified=NOW() WHERE key=$5", collection.group.zot.dbSchema)
	params := []interface{}{
		collection.Version,
		SyncStatusString[collection.Status],
		data,
		collection.Deleted,
		collection.Key,
	}
	_, err = collection.group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, data, "", "\t")
	if err != nil {
		return emperror.Wrapf(err, "cannot pretty json")
	}
	gcommit := fmt.Sprintf("%v - %v.%v v%v", collection.Data.Name, collection.group.Id, collection.Key, collection.Version)
	var fname string
	fname = fmt.Sprintf("%v/collections/%v.json", collection.group.Id, collection.Key)
	if err := collection.group.zot.uploadGitlab(fname, "master", gcommit, "", prettyJSON.String()); err != nil {
		return emperror.Wrapf(err, "update on gitlab failed")
	}
	return nil
}

func (collection *Collection) UpdateCloud() error {
	collection.group.zot.logger.Infof("Creating Zotero Collection [#%s]", collection.Key)

	collection.Data.Version = collection.Version
	if collection.Deleted {
		endpoint := fmt.Sprintf("/groups/%v/collections/%v", collection.group.Id, collection.Key)
		collection.group.zot.logger.Infof("rest call: DELETE %s", endpoint)
		resp, err := collection.group.zot.client.R().
			SetHeader("Accept", "application/json").
			SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", collection.Version)).
			Delete(endpoint)
		if err != nil {
			return emperror.Wrapf(err, "create collection %v with %s", collection.Key, endpoint)
		}
		switch resp.RawResponse.StatusCode {
		case 409:
			return errors.New(fmt.Sprintf("delete: Conflict: the target library #%v is locked", collection.group.Id))
		case 412:
			return errors.New(fmt.Sprintf("delete: Precondition failed: The item #%v.%v has changed since retrieval", collection.group.Id, collection.Key))
		case 428:
			return errors.New(fmt.Sprintf("delete: Precondition required: If-Unmodified-Since-Version was not provided."))
		}
	} else {
		endpoint := fmt.Sprintf("/groups/%v/collections", collection.group.Id)
		collection.group.zot.logger.Infof("rest call: POST %s", endpoint)
		collections := []CollectionData{collection.Data}
		resp, err := collection.group.zot.client.R().
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
			return emperror.Wrapf(err, "could not create item #%v.%v", collection.group.Id, collection.Key)
		}
		if successKey != collection.Key {
			return errors.New(fmt.Sprintf("invalid key %s. source key: %s", successKey, collection.Key))
		}
	}
	collection.Status = SyncStatus_Synced
	if err := collection.UpdateLocal(); err != nil {
		return errors.New(fmt.Sprintf("cannot store item in db %v.%v", collection.group.Id, collection.Key))
	}
	return nil
}
