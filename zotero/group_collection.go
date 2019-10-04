package zotero

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"gopkg.in/resty.v1"
	"strconv"
	"strings"
)

func (group *Group) CreateCollectionLocal(collectionData *CollectionData) (*Collection, error) {
	collectionData.Key = CreateKey()
	coll := &Collection{
		Key:     collectionData.Key,
		Version: 0,
		Library: *group.GetLibrary(),
		Meta:    CollectionMeta{},
		Data:    *collectionData,
		group:   group,
	}
	jsonstr, err := json.Marshal(collectionData)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot marshall collection data %v", collectionData)
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.collections (key, version, library, sync, data, deleted) VALUES( $1, $2, $3, $4, $5, false)", group.zot.dbSchema)
	params := []interface{}{
		coll.Key,
		0,
		coll.Library.Id,
		"new",
		string(jsonstr),
	}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return coll, nil

}

func (group *Group) TryDeleteCollectionLocal(key string, lastModifiedVersion int64) error {
	coll, err := group.GetCollectionByKeyLocal(key)
	if err != nil {
		return emperror.Wrapf(err, "cannot get collection %v", key)
	}
	// no collection, no deletion
	if coll == nil {
		return nil
	}
	if coll.Deleted {
		return nil
	}

	if coll.Status == SyncStatus_Synced {
		// all fine. delete item
		coll.Deleted = true
	} else if group.direction == SyncDirection_ToLocal || group.direction == SyncDirection_BothCloud {
		// cloud leads --> delete
		coll.Deleted = true
		coll.Status = SyncStatus_Synced
	} else {
		// local leads sync back to cloud
		coll.Version = lastModifiedVersion
		coll.Status = SyncStatus_Synced
	}
	if err := coll.UpdateLocal(); err != nil {
		return emperror.Wrapf(err, "cannot update collection %v", key)
	}
	return nil
}

func (group *Group) CreateEmptyCollectionLocal(collectionId string) error {
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

func (group *Group) GetCollectionVersionLocal(collectionId string) (int64, SyncStatus, error) {
	sqlstr := fmt.Sprintf("SELECT version, sync FROM %s.collections WHERE key=$1", group.zot.dbSchema)
	params := []interface{}{
		collectionId,
	}
	var version int64
	var sync string
	var status SyncStatus
	err := group.zot.db.QueryRow(sqlstr, params...).Scan(&version, &sync)
	switch {
	case err == sql.ErrNoRows:
		if err := group.CreateEmptyCollectionLocal(collectionId); err != nil {
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

func (group *Group) GetCollectionsVersionCloud(sinceVersion int64) (*map[string]int64, int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/collections", group.Id)

	var lastModifiedVersion int64
	totalObjects := &map[string]int64{}
	limit := int64(500)
	start := int64(0)
	for {

		group.zot.logger.Infof("rest call: %s [%v, %v]", endpoint, start, limit)

		call := group.zot.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
			SetQueryParam("format", "versions").
			SetQueryParam("limit", strconv.FormatInt(limit, 10)).
			SetQueryParam("start", strconv.FormatInt(start, 10))
		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				return nil, 0, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
			}
			if !group.zot.CheckRetry(resp.Header()) {
				break
			}
		}
		totalResult, err := strconv.ParseInt(resp.RawResponse.Header.Get("Total-Results"), 10, 64)
		if err != nil {
			return nil, 0, emperror.Wrapf(err, "cannot parse Total-Results %v", resp.RawResponse.Header.Get("Total-Results"))
		}
		rawBody := resp.Body()
		objects := &map[string]int64{}
		if err := json.Unmarshal(rawBody, objects); err != nil {
			return nil, 0, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		h, _ := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64)
		if h > lastModifiedVersion {
			lastModifiedVersion = h
		}

		group.zot.CheckBackoff(resp.Header())
		for key, version := range *objects {
			(*totalObjects)[key] = version
		}
		if totalResult <= start+limit {
			break
		}
		start += limit
	}

	return totalObjects, lastModifiedVersion, nil
}

func (group *Group) GetCollectionsCloud(objectKeys []string) (*[]Collection, int64, error) {
	if len(objectKeys) == 0 {
		return &[]Collection{}, 0, errors.New("no objectKeys")
	}
	if len(objectKeys) > 50 {
		return nil, 0, errors.New("too much objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/collections", group.Id)
	group.zot.logger.Infof("rest call: %s", endpoint)

	call := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("collectionKey", strings.Join(objectKeys, ","))

	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, 0, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
		}
		if !group.zot.CheckRetry(resp.Header()) {
			break
		}
	}
	rawBody := resp.Body()
	collections := []Collection{}
	if err := json.Unmarshal(rawBody, &collections); err != nil {
		return nil, 0, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	lastModifiedVersion, err := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64)

	group.zot.CheckBackoff(resp.Header())
	result := []Collection{}
	for _, coll := range collections {
		if coll.Library.Type != "group" {
			return nil, 0, errors.New(fmt.Sprintf("unknown library type %v for collection %v", coll.Library.Type, coll.Key))
		}
		if coll.Library.Id != group.Id {
			return nil, 0, errors.New(fmt.Sprintf("wrong library id %v for collection %v - current group is %v", coll.Library.Id, coll.Key, group.Id))
		}
		coll.group = group
		result = append(result, coll)
	}
	return &result, lastModifiedVersion, nil
}

func (group *Group) syncModifiedCollections() (int64, error) {
	var counter int64
	sqlstr := fmt.Sprintf("SELECT key, version, data, deleted, sync"+
		" FROM %s.collections"+
		" WHERE library=$1 AND (sync=$2 or sync=$3)", group.zot.dbSchema)
	params := []interface{}{
		group.Id,
		"new",
		"modified",
	}
	rows, err := group.zot.db.Query(sqlstr, params...)
	if err != nil {
		return 0, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()
	for rows.Next() {
		collection := Collection{}
		var datastr sql.NullString
		var sync string
		if err := rows.Scan(&collection.Key, &collection.Version, &datastr, &collection.Deleted, &sync); err != nil {
			return 0, emperror.Wrapf(err, "cannot scan result %s: %v", sqlstr, params)
		}
		collection.Status = SyncStatusId[sync]
		if datastr.Valid {
			if err := json.Unmarshal([]byte(datastr.String), &collection.Data); err != nil {
				return 0, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
			}
		} else {
			return 0, emperror.Wrapf(err, "item has no data %v.%v", group.Id, collection.Key)
		}
		collection.group = group
		if err := collection.UpdateCloud(); err != nil {
			return 0, emperror.Wrapf(err, "error creating/updating item %v.%v", group.Id, collection.Key)
		}
		counter++
	}
	return counter, nil
}

func (group *Group) SyncCollections() (int64, int64, error) {
	var lastModifiedVersion int64
	var num int64
	var num2 int64
	var err error

	// upload data
	if group.CanUpload() {
		num, err = group.syncModifiedCollections()
		if err != nil {
			return 0, 0, err
		}
	}

	if group.CanDownload() {
		num2, lastModifiedVersion, err = group.syncCollections()
		if err != nil {
			return 0, 0, err
		}
	}
	counter := num + num2
	if counter > 0 {
		group.zot.logger.Infof("refreshing materialized view collection_name_hier")
		sqlstr := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s.collection_name_hier WITH DATA", group.zot.dbSchema)
		_, err := group.zot.db.Exec(sqlstr)
		if err != nil {
			return counter, 0, emperror.Wrapf(err, "cannot refresh materialized view item_type_hier - %v", sqlstr)
		}
	}

	return counter, lastModifiedVersion, nil
}

func (group *Group) syncCollections() (int64, int64, error) {
	group.zot.logger.Infof("Syncing collections of group #%v", group.Id)
	var counter int64
	objectList, lastModifiedVersion, err := group.GetCollectionsVersionCloud(group.CollectionVersion)
	if err != nil {
		return counter, 0, emperror.Wrapf(err, "cannot get collection versions")
	}
	collectionUpdate := []string{}
	for collectionid, version := range *objectList {
		oldversion, status, err := group.GetCollectionVersionLocal(collectionid)
		if err != nil {
			return counter, 0, emperror.Wrapf(err, "cannot get version of collection %v from database: %v", collectionid, err)
		}
		if status != SyncStatus_Synced && status != SyncStatus_New {
			return counter, 0, errors.New(fmt.Sprintf("collection %v not synced. please handle conflict", collectionid))
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
		if len(part) > 0 {
			colls, h, err := group.GetCollectionsCloud(part)
			if err != nil {
				return counter, 0, emperror.Wrapf(err, "cannot get collections")
			}
			if h > lastModifiedVersion {
				lastModifiedVersion = h
			}
			group.zot.logger.Infof("%v collections", len(*colls))
			for _, coll := range *colls {
				coll.Status = SyncStatus_Synced
				if err := coll.UpdateLocal(); err != nil {
					return counter, 0, emperror.Wrapf(err, "cannot update collection")
				}
				counter++
			}
		}
	}
	return counter, lastModifiedVersion, nil
}

func (group *Group) GetCollectionByNameLocal(name string, parentKey string) (*Collection, error) {

	coll := Collection{
		Key:     "",
		Version: 0,
		Library: Library{},
		Links:   nil,
		Meta:    CollectionMeta{},
		Data:    CollectionData{},
		Status:  SyncStatus_New,
	}

	sqlstr := fmt.Sprintf("SELECT cs.key,cs.version,cs.data,cs.meta,cs.deleted,cs.sync"+
		" FROM %s.collections cs, %s.collection_name_hier cnh"+
		" WHERE cs.key=cnh.key AND cs.library=$1 AND cnh.name=$2", group.zot.dbSchema, group.zot.dbSchema)

	params := []interface{}{
		group.Id,
		name,
	}
	if parentKey != "" {
		sqlstr += " AND cnh.parent=$3"
		params = append(params, parentKey)
	} else {
		sqlstr += " AND cnh.parent IS NULL"
	}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	if err := group.zot.db.QueryRow(sqlstr, params...).
		Scan(&coll.Key, &coll.Version, &datastr, &metastr, &coll.Deleted, &sync); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, emperror.Wrapf(err, "cannot get collection: %v - %v", sqlstr, params)
	}
	coll.Status = SyncStatusId[sync]
	if err := json.Unmarshal([]byte(datastr.String), &coll.Data); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshall collection data - %v", datastr)
	}
	if err := json.Unmarshal([]byte(metastr.String), &coll.Meta); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshall collection metadata - %v", metastr)
	}

	return &coll, nil
}

func (group *Group) GetCollectionByKeyLocal(key string) (*Collection, error) {

	coll := Collection{
		Key:     "",
		Version: 0,
		Library: Library{},
		Links:   nil,
		Meta:    CollectionMeta{},
		Data:    CollectionData{},
		Status:  SyncStatus_New,
	}

	sqlstr := fmt.Sprintf("SELECT cs.key,cs.version,cs.data,cs.meta,cs.deleted,cs.sync"+
		" FROM %s.collections cs WHERE cs.library=$1 AND cs.key=$2", group.zot.dbSchema)

	params := []interface{}{
		group.Id,
		key,
	}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	if err := group.zot.db.QueryRow(sqlstr, params...).
		Scan(&coll.Key, &coll.Version, &datastr, &metastr, &coll.Deleted, &sync); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, emperror.Wrapf(err, "cannot get collection: %v - %v", sqlstr, params)
	}
	coll.Status = SyncStatusId[sync]
	if err := json.Unmarshal([]byte(datastr.String), &coll.Data); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshall collection data - %v", datastr)
	}
	if err := json.Unmarshal([]byte(metastr.String), &coll.Meta); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshall collection metadata - %v", metastr)
	}

	return &coll, nil
}
