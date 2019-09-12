package zotero

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"strconv"
	"strings"
)

type CollectionLibrary struct {
	Type  string      `json:"type"`
	Id    int64       `json:"id"`
	Name  string      `json:"name"`
	Links interface{} `json:"links"`
}

type Collection struct {
	Key     string                 `json:"key"`
	Version int64                  `json:"version"`
	Library CollectionLibrary      `json:"library,omitempty"`
	Links   interface{}            `json:"links,omitempty"`
	Meta    interface{}            `json:"meta,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func (group *Group) CreateCollectionDB(collectionId string) (error) {
	sqlstr := fmt.Sprintf("INSERT INTO %s.collections (key, version, library, synced) VALUES( $1, 0, $2, true)", group.zot.dbSchema)
	params := []interface{}{
		collectionId,
		group.Id,
	}
	_, err := group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}



func (zot *Zotero) DeleteCollectionDB(key string) error {
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET deleted=true WHERE key=$1", zot.dbSchema)

	params := []interface{}{
		key,
	}
	if _, err := zot.db.Exec(sqlstr, params...); err != nil {
		return emperror.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetCollectionVersionDB(collectionId string) (int64, bool, error) {
	sqlstr := fmt.Sprintf( "SELECT version, synced FROM %s.collections WHERE key=$1", group.zot.dbSchema)
	params := []interface{}{
		collectionId,
	}
	var version int64
	var synced bool
	err := group.zot.db.QueryRow(sqlstr, params...).Scan(&version, &synced)
	switch {
	case err == sql.ErrNoRows:
		if err := group.CreateCollectionDB(collectionId); err != nil {
			return 0, false, emperror.Wrapf(err, "cannot create new collection")
		}
		version = 0
		synced = true
	case err != nil:
		return 0, false, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return version, synced, nil
}

func (group *Group) UpdateCollectionDB(collection *Collection) error {
	group.zot.logger.Infof("Updating Collection [#%s]", collection.Key)
	data, err := json.Marshal(collection.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", collection.Data)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET version=$1, synced=true, data=$2 WHERE key=$3", group.zot.dbSchema)
	params := []interface{}{
		collection.Version,
		data,
		collection.Key,
	}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetCollectionsVersion(sinceVersion int64) (*map[string]int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections", group.Id)

	totalObjects := &map[string]int64{}
	limit := int64(500)
	start := int64(0)
	for {

		group.zot.logger.Infof("rest call: %s [%v, %v]", endpoint, start, limit)

		resp, err := group.zot.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
			SetQueryParam("format", "versions").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10)).
			Get(endpoint)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
		}
		totalResult, err := strconv.ParseInt(resp.RawResponse.Header.Get("Total-Results"), 10, 64)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot parse Total-Results %v", resp.RawResponse.Header.Get("Total-Results"))
		}
		rawBody := resp.Body()
		objects := &map[string]int64{}
		if err := json.Unmarshal(rawBody, objects); err != nil {
			return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		for key, version := range *objects {
			(*totalObjects)[key] = version
		}
		if totalResult <= start+limit {
			break
		}
		start += limit
	}
	return totalObjects, nil
}

func (group *Group) GetCollections(objectKeys []string) (*[]Collection, error) {
	if len(objectKeys) == 0 {
		return &[]Collection{}, nil
	}
	if len(objectKeys) > 50 {
		return nil, errors.New("too much objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/collections", group.Id)
	group.zot.logger.Infof("rest call: %s", endpoint)

	resp, err := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("collectionKey", strings.Join(objectKeys, ",")).
		Get(endpoint)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	collections := &[]Collection{}
	if err := json.Unmarshal(rawBody, collections); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return collections, nil
}

func (group *Group) SyncCollections() (int64, error) {
	group.zot.logger.Infof("Syncing collections of group #%v", group.Id)
	var counter int64
	objectList, err := group.GetCollectionsVersion(group.Version)
	if err != nil {
		return counter, emperror.Wrapf(err, "cannot get collection versions")
	}
	collectionUpdate := []string{}
	for collectionid, version := range *objectList {
		oldversion, synced, err := group.GetCollectionVersionDB(collectionid)
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot get version of collection %v from database: %v", collectionid, err )
		}
		if !synced {
			return counter, errors.New(fmt.Sprintf("collection %v not synced. please handle conflict", collectionid))
		}
		if oldversion < version {
			collectionUpdate = append(collectionUpdate, collectionid)
		}
	}
	numColls := len(collectionUpdate)
	for i := 0; i < (numColls/50)+1; i++ {
		start := i * 50
		end := start + 50
		if numColls < end {
			end = numColls
		}
		part := collectionUpdate[start:end]
		colls, err := group.GetCollections(part)
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot get collections")
		}
		group.zot.logger.Infof("%v collections", len(*colls))
		for _, coll := range *colls {
			if err := group.UpdateCollectionDB(&coll); err != nil {
				return counter, emperror.Wrapf(err, "cannot update collection")
			}
			counter++
		}
	}
	group.zot.logger.Infof("Syncing collections of group #%v done. %v collections changed", group.Id, counter)
	return counter, nil
}

