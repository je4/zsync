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

func (group *Group) CreateCollectionDB(collectionId string) (error) {
	sqlstr := fmt.Sprintf("INSERT INTO %s.collections (key, version, library, sync) VALUES( $1, 0, $2, $3)", group.zot.dbSchema)
	params := []interface{}{
		collectionId,
		group.Id,
		SyncStatusString[SyncStatus_New],
	}
	_, err := group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetCollectionVersionDB(collectionId string) (int64, SyncStatus, error) {
	sqlstr := fmt.Sprintf( "SELECT version, sync FROM %s.collections WHERE key=$1", group.zot.dbSchema)
	params := []interface{}{
		collectionId,
	}
	var version int64
	var sync string
	var status SyncStatus
	err := group.zot.db.QueryRow(sqlstr, params...).Scan(&version, &sync)
	switch {
	case err == sql.ErrNoRows:
		if err := group.CreateCollectionDB(collectionId); err != nil {
			return 0, SyncStatus_Incomplete, emperror.Wrapf(err, "cannot create new collection")
		}
		version = 0
		status = SyncStatus_New
	case err != nil:
		return 0, SyncStatus_Incomplete, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	case err == nil:
		status = SyncStatusId[sync]
	}
	return version, status, nil
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
		group.zot.CheckWait(resp.Header())
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
	collections := []Collection{}
	if err := json.Unmarshal(rawBody, &collections); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	group.zot.CheckWait(resp.Header())
	result := []Collection{}
	for _, coll := range collections {
		if coll.Library.Type != "group" {
			return nil, errors.New(fmt.Sprintf("unknown library type %v for collection %v", coll.Library.Type, coll.Key))
		}
		if coll.Library.Id != group.Id {
			return nil, errors.New(fmt.Sprintf("wrong library id %v for collection %v - current group is %v", coll.Library.Id, coll.Key, group.Id))
		}
		coll.group = group
		result = append(result, coll)
	}
	return &result, nil
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
		oldversion, status, err := group.GetCollectionVersionDB(collectionid)
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot get version of collection %v from database: %v", collectionid, err )
		}
		if status != SyncStatus_Synced && status != SyncStatus_New {
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
			coll.Status = SyncStatus_Synced
			if err := coll.UpdateCollectionDB(); err != nil {
				return counter, emperror.Wrapf(err, "cannot update collection")
			}
			counter++
		}
	}
	group.zot.logger.Infof("Syncing collections of group #%v done. %v collections changed", group.Id, counter)
	return counter, nil
}


