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

func (group *Group) DeleteItemDB(key string) error {
	sqlstr := fmt.Sprintf("UPDATE %s.items SET deleted=true WHERE key=$1 AND library=$2", group.zot.dbSchema)

	params := []interface{}{
		key,
		group.Id,
	}
	if _, err := group.zot.db.Exec(sqlstr, params...); err != nil {
		return emperror.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) CreateItemDB(itemData *ItemGeneric, itemMeta *ItemMeta, oldId string) (*Item, error) {
	itemData.Key = CreateKey()
	item := &Item{
		Key:     itemData.Key,
		Version: 0,
		Library: *group.GetLibrary(),
		Meta:    *itemMeta,
		Data:    *itemData,
		group:   group,
		OldId:   oldId,
		Status:SyncStatus_New,
	}
	jsonstr, err := json.Marshal(itemData)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot marshall item data %v", itemData)
	}
	oid := sql.NullString{
		String: oldId,
		Valid:  true,
	}
	if oldId == "" {
		oid.Valid = false
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, sync, data, oldid) VALUES( $1, $2, $3, $4, $5, $6)", group.zot.dbSchema)
	params := []interface{}{
		item.Key,
		0,
		item.Library.Id,
		"new",
		string(jsonstr),
		oid,
	}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if IsUniqueViolation(err, "items_oldid_constraint") {
		item2, err := group.GetItemByOldid(oldId)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot load item %v", oldId)
		}
		item.Key = item2.Key
		item.Data.Key = item.Key
		item.Version = item2.Version
		item.Status = SyncStatus_Modified
		err = item.UpdateDB()
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot update item %v", oldId)
		}
	} else if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return item, nil
}

func (group *Group) CreateEmptyItemDB(itemId string, oldId string) error {
	oid := sql.NullString{
		String: oldId,
		Valid:  true,
	}
	if oldId == "" {
		oid.Valid = false
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, sync, oldid) VALUES( $1, 0, $2, $3, $4)", group.zot.dbSchema)
	params := []interface{}{
		itemId,
		group.Id,
		"incomplete",
		oid,
	}
	_, err := group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetItemVersionDB(itemId string, oldId string) (int64, SyncStatus, error) {
	sqlstr := fmt.Sprintf("SELECT version, sync FROM %s.items WHERE library=$1 AND key=$2", group.zot.dbSchema)
	params := []interface{}{
		group.Id,
		itemId,
	}
	var version int64
	var syncstr string
	var sync SyncStatus
	err := group.zot.db.QueryRow(sqlstr, params...).Scan(&version, &syncstr)
	switch {
	case err == sql.ErrNoRows:
		if err := group.CreateEmptyItemDB(itemId, oldId); err != nil {
			return 0, SyncStatus_New, emperror.Wrapf(err, "cannot create new collection")
		}
		version = 0
		sync = SyncStatus_Synced
	case err != nil:
		return 0, SyncStatus_New, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	switch syncstr {
	case "new":
		sync = SyncStatus_New
	case "synced":
		sync = SyncStatus_Synced
	case "modified":
		sync = SyncStatus_Modified
	case "incomplete":
		sync = SyncStatus_Incomplete
	}
	return version, sync, nil
}

func (group *Group) GetItemsVersion(sinceVersion int64, trashed bool) (*map[string]int64, error) {
	var endpoint string
	if trashed {
		endpoint = fmt.Sprintf("/groups/%v/items/trash", group.Id)
	} else {
		endpoint = fmt.Sprintf("/groups/%v/items", group.Id)
	}

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
				return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
			}
			if !group.zot.CheckRetry(resp.Header()) {
				break
			}
		}
		rawBody := resp.Body()
		objects := &map[string]int64{}
		if err := json.Unmarshal(rawBody, objects); err != nil {
			return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		totalResult, err := strconv.ParseInt(resp.RawResponse.Header.Get("Total-Results"), 10, 64)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot parse Total-Results %v", resp.RawResponse.Header.Get("Total-Results"))
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
	return totalObjects, nil
}

func (group *Group) GetItemsDB(objectKeys []string) (*[]Item, error) {
	if len(objectKeys) == 0 {
		return &[]Item{}, nil
	}
	params := []interface{}{
		group.Id,
	}
	pstr := []string{}
	for _, val := range objectKeys {
		params = append(params, val)
		pstr = append(pstr, fmt.Sprintf("$%v", len(params)))
	}
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5 FROM %s.items WHERE library=$1 AND key IN (%s)", group.zot.dbSchema, strings.Join(pstr, ","))
	rows, err := group.zot.db.Query(sqlstr, params...)
	if err != nil {
		return &[]Item{}, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()

	result := []Item{}
	for rows.Next() {
		item := Item{}
		var datastr sql.NullString
		var sync string
		var md5str sql.NullString
		if err := rows.Scan(&item.Key, &item.Version, &datastr, &item.Trashed, &item.Deleted, &sync, &md5str); err != nil {
			return &[]Item{}, emperror.Wrapf(err, "cannot scan result %s: %v", sqlstr, params)
		}
		if md5str.Valid {
			item.MD5 = md5str.String
		}
		item.Status = SyncStatusId[sync]
		if datastr.Valid {
			if err := json.Unmarshal([]byte(datastr.String), &item.Data); err != nil {
				return &[]Item{}, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
			}
			if item.Data.Collections == nil {
				item.Data.Collections = []string{}
			}
		} else {
			return &[]Item{}, emperror.Wrapf(err, "item has no data %v.%v", group.Id, item.Key)
		}
		item.group = group
		result = append(result, item)
	}
	return &result, nil
}
func (group *Group) GetItems(objectKeys []string) (*[]Item, error) {
	if len(objectKeys) == 0 {
		return &[]Item{}, nil
	}
	if len(objectKeys) > 50 {
		return nil, errors.New("too much objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/items", group.Id)
	group.zot.logger.Infof("rest call: %s", endpoint)

	call := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("itemKey", strings.Join(objectKeys, ",")).
		SetQueryParam("includeTrashed", "1")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
		}
		if !group.zot.CheckRetry(resp.Header()) {
			break
		}
	}
	rawBody := resp.Body()
	items := []Item{}
	if err := json.Unmarshal(rawBody, &items); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	group.zot.CheckBackoff(resp.Header())
	result := []Item{}
	for _, item := range items {
		item.group = group
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
		result = append(result, item)
	}
	return &result, nil
}

func (group *Group) SyncItems() (int64, error) {
	num, err := group.syncNewItems()
	if err != nil {
		return 0, err
	}
	num2, err := group.syncItems(true)
	if err != nil {
		return 0, err
	}
	num3, err := group.syncItems(false)
	if err != nil {
		return 0, err
	}
	counter := num + num2 + num3
	if counter > 0 {
		group.zot.logger.Infof("refreshing materialized view item_type_hier")
		sqlstr := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s.item_type_hier WITH DATA", group.zot.dbSchema)
		_, err := group.zot.db.Exec(sqlstr)
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot refresh materialized view item_type_hier - %v", sqlstr)
		}
	}

	return counter, nil
}

func (group *Group) syncNewItems() (int64, error) {
	var counter int64
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5 FROM %s.items WHERE library=$1 AND (sync=$2 or sync=$3)", group.zot.dbSchema)
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
		item := Item{}
		var datastr sql.NullString
		var sync string
		var md5str sql.NullString
		if err := rows.Scan(&item.Key, &item.Version, &datastr, &item.Trashed, &item.Deleted, &sync, &md5str); err != nil {
			return 0, emperror.Wrapf(err, "cannot scan result %s: %v", sqlstr, params)
		}
		if md5str.Valid {
			item.MD5 = md5str.String
		}
		item.Status = SyncStatusId[sync]
		if datastr.Valid {
			if err := json.Unmarshal([]byte(datastr.String), &item.Data); err != nil {
				return 0, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
			}
			if item.Data.Collections == nil {
				item.Data.Collections = []string{}
			}
		} else {
			return 0, emperror.Wrapf(err, "item has no data %v.%v", group.Id, item.Key)
		}
		item.group = group
		if err := item.Update(); err != nil {
			return 0, emperror.Wrapf(err, "error creating/updating item %v.%v", group.Id, item.Key)
		}
		counter++
	}
	return counter, nil
}

func (group *Group) syncItems(trashed bool) (int64, error) {
	group.zot.logger.Infof("Syncing items of group #%v", group.Id)
	var counter int64
	objectList, err := group.GetItemsVersion(group.Version, trashed)
	if err != nil {
		return counter, emperror.Wrapf(err, "cannot get collection versions")
	}
	itemsUpdate := []string{}
	for itemid, version := range *objectList {
		oldversion, sync, err := group.GetItemVersionDB(itemid, "")
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot get version of item %v from database: %v", itemid, err)
		}
		if sync != SyncStatus_Synced && sync != SyncStatus_Incomplete {
			return counter, errors.New(fmt.Sprintf("item %v not synced. please handle conflict", itemid))
		}
		if oldversion < version {
			itemsUpdate = append(itemsUpdate, itemid)
		}
	}
	numItems := len(itemsUpdate)
	for i := 0; i < (numItems/50)+1; i++ {
		start := i * 50
		end := start + 50
		if numItems < end {
			end = numItems
		}
		part := itemsUpdate[start:end]
		items, err := group.GetItems(part)
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot get items")
		}
		group.zot.logger.Infof("%v items", len(*items))
		for _, item := range *items {
			group.zot.logger.Infof("Item %v of %v", counter, numItems)
			item.Status = SyncStatus_Synced
			if err := item.UpdateDB(); err != nil {
				return counter, emperror.Wrapf(err, "cannot update items")
			}
			counter++
		}
	}
	group.zot.logger.Infof("Syncing items of group #%v done. %v items changed", group.Id, counter)
	return counter, nil
}

func (group *Group) GetItemByKey(key string) (*Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5 FROM %s.items" +
		" WHERE library=$1 AND key=$2 AND deleted=false", group.zot.dbSchema)
	params := []interface{}{
		group.Id,
		key,
	}
	item := Item{}
	var datastr sql.NullString
	var sync string
	var md5str sql.NullString
	if err := group.zot.db.QueryRow(sqlstr, params...).
		Scan(&item.Key, &item.Version, &datastr, &item.Trashed, &item.Deleted, &sync, &md5str); err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	if md5str.Valid {
		item.MD5 = md5str.String
	}
	item.Status = SyncStatusId[sync]
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &item.Data); err != nil {
			return nil, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
		}
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
	} else {
		return nil, errors.New(fmt.Sprintf("item has no data %v.%v", group.Id, item.Key))
	}
	item.group = group

	return &item, nil
}


func (group *Group) GetItemByOldid(oldid string) (*Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5 FROM %s.items WHERE library=$1 AND oldid=$2 AND deleted=false", group.zot.dbSchema)
	params := []interface{}{
		group.Id,
		oldid,
	}
	item := Item{}
	var datastr sql.NullString
	var sync string
	var md5str sql.NullString
	if err := group.zot.db.QueryRow(sqlstr, params...).
		Scan(&item.Key, &item.Version, &datastr, &item.Trashed, &item.Deleted, &sync, &md5str); err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	if md5str.Valid {
		item.MD5 = md5str.String
	}
	item.Status = SyncStatusId[sync]
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &item.Data); err != nil {
			return nil, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
		}
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
	} else {
		return nil, errors.New(fmt.Sprintf("item has no data %v.%v", group.Id, item.Key))
	}
	item.group = group

	return &item, nil
}
