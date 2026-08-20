package zotero

import (
	"emperror.dev/errors"
	"encoding/json"
	"fmt"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"gopkg.in/resty.v1"
	"path/filepath"
	"time"
)

type CollectionData struct {
	Key              string       `json:"key,omitempty"`
	Name             string       `json:"name"`
	Version          int64        `json:"version,omitempty"`
	Relations        RelationList `json:"relations"`
	ParentCollection Parent       `json:"parentCollection"`
}

type CollectionMeta struct {
	NumCollections int64 `json:"numCollections"`
	NumItems       int64 `json:"numItems"`
}

type Collection struct {
	Key     string         `json:"key"`
	Version int64          `json:"version"`
	Library Library        `json:"library"`
	Links   any            `json:"links,omitempty"`
	Meta    CollectionMeta `json:"meta"`
	Data    CollectionData `json:"data"`
	Group   *Group         `json:"-"`
	Status  SyncStatus     `json:"-"`
	Trashed bool           `json:"-"`
	Deleted bool           `json:"-"`
	Gitlab  *time.Time     `json:"-"`
}

type CollectionGitlab struct {
	LibraryId int64          `json:"libraryid"`
	Key       string         `json:"key"`
	Data      CollectionData `json:"data"`
	Meta      CollectionMeta `json:"meta"`
}

func (collection *Collection) UpdateLocal() error {
	collection.Group.Zot.Logger.Info().Msgf("Updating Collection [#%s]", collection.Key)
	data, err := json.Marshal(collection.Data)
	if err != nil {
		return errors.Wrapf(err, "cannot marshall data %v", collection.Data)
	}
	meta, err := json.Marshal(collection.Meta)
	if err != nil {
		return errors.Wrapf(err, "cannot marshall meta %v", collection.Meta)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET version=$1, sync=$2, data=$3, meta=$4, deleted=$5, modified=NOW() WHERE key=$6", collection.Group.Zot.dbSchema)
	params := []any{
		collection.Version,
		SyncStatusString[collection.Status],
		data,
		meta,
		collection.Deleted,
		collection.Key,
	}
	_, err = collection.Group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (collection *Collection) UpdateCloud() error {
	collection.Group.Zot.Logger.Info().Msgf("Creating Zotero Collection [#%s]", collection.Key)

	collection.Data.Version = collection.Version
	if collection.Deleted {
		endpoint := fmt.Sprintf("/groups/%v/collections/%v", collection.Group.Id, collection.Key)
		collection.Group.Zot.Logger.Info().Msgf("rest call: DELETE %s", endpoint)
		var resp *resty.Response
		var err error
		for {
			resp, err = collection.Group.Zot.client.R().
				SetHeader("Accept", "application/json").
				SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", collection.Version)).
				Delete(endpoint)
			if err != nil {
				return errors.Wrapf(err, "create collection %v with %s", collection.Key, endpoint)
			}
			if !collection.Group.Zot.CheckRetry(resp.Header()) {
				break
			}
		}
		collection.Group.Zot.CheckBackoff(resp.Header())
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
		collection.Group.Zot.Logger.Info().Msgf("rest call: POST %s", endpoint)
		sendData := collection.Data
		if collection.Version == 0 {
			sendData.Key = ""
		}
		collections := []CollectionData{sendData}
		var resp *resty.Response
		var err error
		for {
			resp, err = collection.Group.Zot.client.R().
				SetHeader("Accept", "application/json").
				SetBody(collections).
				Post(endpoint)
			if err != nil {
				return errors.Wrapf(err, "create collection %v with %s", collection.Key, endpoint)
			}
			if !collection.Group.Zot.CheckRetry(resp.Header()) {
				break
			}
		}
		collection.Group.Zot.CheckBackoff(resp.Header())
		if resp.StatusCode() >= 400 {
			return errors.Errorf("create collection failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
		}
		result := ItemCollectionCreateResult{}
		jsonstr := resp.Body()
		if err := json.Unmarshal(jsonstr, &result); err != nil {
			return errors.Wrapf(err, "cannot unmarshall result %s", string(jsonstr))
		}
		successKey, err := result.checkSuccess(0)
		if err != nil {
			return errors.Wrapf(err, "could not create item #%v.%v", collection.Group.Id, collection.Key)
		}
		collection.Key = successKey
		collection.Data.Key = successKey
		if successfulItem, ok := result.Successful["0"]; ok && successfulItem.Version > 0 {
			collection.Version = successfulItem.Version
			collection.Data.Version = successfulItem.Version
		}
	}
	collection.Status = SyncStatus_Synced
	if collection.Group != nil && collection.Group.Zot != nil && collection.Group.Zot.db != nil {
		if err := collection.UpdateLocal(); err != nil {
			return errors.New(fmt.Sprintf("cannot store item in db %v.%v", collection.Group.Id, collection.Key))
		}
	}
	return nil
}

func (collection *Collection) DeleteCloud(lastModifiedVersion int64) error {
	endpoint := fmt.Sprintf("/groups/%v/collections/%v", collection.Group.Id, collection.Key)
	collection.Group.Zot.Logger.Info().Msgf("rest call: DELETE %s", endpoint)
	req := collection.Group.Zot.client.R().
		SetHeader("Accept", "application/json")
	if lastModifiedVersion > 0 {
		req.SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", lastModifiedVersion))
	}
	var resp *resty.Response
	var err error
	for {
		resp, err = req.Delete(endpoint)
		if err != nil {
			return errors.Wrapf(err, "delete collection %v with %s", collection.Key, endpoint)
		}
		if !collection.Group.Zot.CheckRetry(resp.Header()) {
			break
		}
	}
	collection.Group.Zot.CheckBackoff(resp.Header())
	switch resp.RawResponse.StatusCode {
	case 200, 204:
		collection.Deleted = true
		collection.Status = SyncStatus_Synced
		if collection.Group != nil && collection.Group.Zot != nil && collection.Group.Zot.db != nil {
			return collection.UpdateLocal()
		}
		return nil
	case 409:
		return errors.New(fmt.Sprintf("delete: Conflict: the target library #%v is locked", collection.Group.Id))
	case 412:
		return errors.New(fmt.Sprintf("delete: Precondition failed: The item #%v.%v has changed since retrieval", collection.Group.Id, collection.Key))
	case 428:
		return errors.New(fmt.Sprintf("delete: Precondition required: If-Unmodified-Since-Version was not provided."))
	default:
		return errors.New(fmt.Sprintf("delete collection failed with status code %d", resp.RawResponse.StatusCode))
	}
}

func (collection *Collection) Backup(backupFs filesystem.FileSystem) error {
	collection.Group.Zot.Logger.Info().Msgf("storing %v to %v", collection.Data.Name, backupFs.String())
	var fname string
	var folder string
	folder = filepath.Clean(fmt.Sprintf("%v/collections", collection.Group.Id))
	fname = filepath.Clean(fmt.Sprintf("%v.json", collection.Key))

	// write data to file
	data := struct {
		LibraryId int64  `json:"libraryid"`
		Id        string `json:"id"`
		Data      any    `json:"data"`
		Meta      any    `json:"meta"`
	}{
		LibraryId: collection.Group.Id,
		Id:        collection.Key,
		Data:      collection.Data,
		Meta:      collection.Meta,
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "cannot marshal data %v", data)
	}
	if err := backupFs.FilePut(folder, fname, b, filesystem.FilePutOptions{}); err != nil {
		return errors.Wrap(err, "cannot write data to file")
	}

	return nil
}
