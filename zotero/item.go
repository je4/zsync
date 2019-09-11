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

type ItemMeta struct {
	CreatedByUser  User   `json:"createdByUser"`
	CreatorSummary string `json:creatorSummary,omitempty`
	ParsedDate     string `json:"parsedDate,omitempty"`
	NumChildren    int64  `json:"numChildren,omitempty"`
}

type Item struct {
	Key     string                 `json:"key"`
	Version int64                  `json:"version"`
	Library CollectionLibrary      `json:"library,omitempty"`
	Links   interface{}            `json:"links,omitempty"`
	Meta    ItemMeta               `json:"meta,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func (group *Group) CreateItemDB(itemId string) (error) {
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, synced) VALUES( $1, 0, $2, true)", group.zot.dbSchema)
	params := []interface{}{
		itemId,
		group.Id,
	}
	_, err := group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetItemVersionDB(itemId string) (int64, bool, error) {
	sqlstr := fmt.Sprintf( "SELECT version, synced FROM %s.items WHERE key=$1", group.zot.dbSchema)
	params := []interface{}{
		itemId,
	}
	var version int64
	var synced bool
	err := group.zot.db.QueryRow(sqlstr, params...).Scan(&version, &synced)
	switch {
	case err == sql.ErrNoRows:
		if err := group.CreateItemDB(itemId); err != nil {
			return 0, false, emperror.Wrapf(err, "cannot create new collection")
		}
		version = 0
		synced = true
	case err != nil:
		return 0, false, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return version, synced, nil
}

func (group *Group) UpdateItemDB(item *Item, trashed bool) error {
	group.zot.logger.Infof("Updating Item [#%s]", item.Key)
	data, err := json.Marshal(item.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", item.Data)
	}
	meta, err := json.Marshal(item.Meta)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall meta %v", item.Meta)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.items SET version=$1, synced=true, data=$2, meta=$3, trashed=$4 WHERE key=$5", group.zot.dbSchema)
	params := []interface{}{
		item.Version,
		data,
		meta,
		trashed,
		item.Key,
	}
	_, err = group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetItemsVersion(sinceVersion int64, trashed bool) (*map[string]int64, error) {

	var endpoint string
	if trashed {
		endpoint = fmt.Sprintf("/groups/%v/items/trash", group.Id)
	} else {
		endpoint = fmt.Sprintf("/groups/%v/items", group.Id)
	}
	group.zot.logger.Infof("rest call: %s", endpoint)

	resp, err := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(sinceVersion, 10)).
		SetQueryParam("format", "versions").
		Get(endpoint)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	objects := &map[string]int64{}
	if err := json.Unmarshal(rawBody, objects); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return objects, nil
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

	resp, err := group.zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("itemKey", strings.Join(objectKeys, ",")).
		Get(endpoint)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	items := &[]Item{}
	if err := json.Unmarshal(rawBody, items); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return items, nil
}

func (group *Group) SyncItems() (int64, error) {
	num, err := group.SyncItems2(true)
	if err != nil {
		return 0, err
	}
	num2, err := group.SyncItems2(false)
	if err != nil {
		return 0, err
	}
	return num+num2, nil
}

func (group *Group) SyncItems2(trashed bool) (int64, error) {
	group.zot.logger.Infof("Syncing items of group #%v", group.Id)
	var counter int64
	objectList, err := group.GetItemsVersion(group.Version, trashed)
	if err != nil {
		return counter, emperror.Wrapf(err, "cannot get collection versions")
	}
	itemsUpdate := []string{}
	for itemid, version := range *objectList {
		oldversion, synced, err := group.GetItemVersionDB(itemid)
		if err != nil {
			return counter, emperror.Wrapf(err, "cannot get version of item %v from database: %v", itemid, err )
		}
		if !synced {
			return counter, errors.New(fmt.Sprintf("iem %v not synced. please handle conflict", itemid))
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
			if err := group.UpdateItemDB(&item, trashed); err != nil {
				return counter, emperror.Wrapf(err, "cannot update collection")
			}
			counter++
		}
	}
	group.zot.logger.Infof("Syncing items of group #%v done. %v items changed", group.Id, counter)
	return counter, nil
}

