package zotero

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/xanzy/go-gitlab"
	"gopkg.in/resty.v1"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func (group *Group) DeleteItemLocal(key string) error {
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

func (group *Group) CreateItemLocal(itemData *ItemGeneric, itemMeta *ItemMeta, oldId string) (*Item, error) {
	itemData.Key = CreateKey()
	item := &Item{
		Key:     itemData.Key,
		Version: 0,
		Library: *group.GetLibrary(),
		Meta:    *itemMeta,
		Data:    *itemData,
		group:   group,
		OldId:   oldId,
		Status:  SyncStatus_New,
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
		item2, err := group.GetItemByOldidLocal(oldId)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot load item %v", oldId)
		}
		item.Key = item2.Key
		item.Data.Key = item.Key
		item.Version = item2.Version
		item.Status = SyncStatus_Modified
		err = item.UpdateLocal()
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot update item %v", oldId)
		}
	} else if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	gcontent := string(jsonstr)
	var gusername string
	if itemMeta != nil {
		gusername = itemMeta.CreatedByUser.Username
	}
	gopt := gitlab.CreateFileOptions{
		Branch:        nil,
		Encoding:      nil,
		AuthorEmail:   nil,
		AuthorName:    &gusername,
		Content:       &gcontent,
		CommitMessage: &itemMeta.CreatorSummary,
	}
	fileinfo, _, err := item.group.zot.git.RepositoryFiles.CreateFile(item.group.zot.gitProject.ID, fmt.Sprintf("%v/%v", item.group.Id, item.Key), &gopt)
	if err != nil {
		return nil, emperror.Wrapf(err, "upload to gitlab failed")
	}
	item.group.zot.logger.Debugf("upload to gitlab done: %v", fileinfo.String())

	return item, nil
}

func (group *Group) CreateEmptyItemLocal(itemId string, oldId string) error {
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

func (group *Group) GetItemVersionLocal(itemId string, oldId string) (int64, SyncStatus, error) {
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
		if err := group.CreateEmptyItemLocal(itemId, oldId); err != nil {
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

func (group *Group) GetItemsVersionCloud(sinceVersion int64, trashed bool) (*map[string]int64, int64, error) {
	var endpoint string
	if trashed {
		endpoint = fmt.Sprintf("/groups/%v/items/trash", group.Id)
	} else {
		endpoint = fmt.Sprintf("/groups/%v/items", group.Id)
	}

	totalObjects := &map[string]int64{}
	limit := int64(500)
	start := int64(0)
	var lastModifiedVersion int64
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
		rawBody := resp.Body()
		objects := &map[string]int64{}
		if err := json.Unmarshal(rawBody, objects); err != nil {
			return nil, 0, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
		}
		totalResult, err := strconv.ParseInt(resp.RawResponse.Header.Get("Total-Results"), 10, 64)
		if err != nil {
			return nil, 0, emperror.Wrapf(err, "cannot parse Total-Results %v", resp.RawResponse.Header.Get("Total-Results"))
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

func (group *Group) GetItemsLocal(objectKeys []string) (*[]Item, error) {
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
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND key IN (%s)", group.zot.dbSchema, strings.Join(pstr, ","))
	rows, err := group.zot.db.Query(sqlstr, params...)
	if err != nil {
		return &[]Item{}, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()

	result := []Item{}
	for rows.Next() {
		item, err := group.itemFromRow(rows)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot scan row")
		}
		result = append(result, *item)
	}
	return &result, nil
}

func (group *Group) GetItemsCloud(objectKeys []string) (*[]Item, error) {
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
	group.zot.logger.Debugf("status: #%v ", resp.StatusCode())
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

func (group *Group) syncModifiedItems() (int64, error) {
	var counter int64
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND (sync=$2 or sync=$3)", group.zot.dbSchema)
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
		item, err := group.itemFromRow(rows)
		if err != nil {
			return 0, emperror.Wrapf(err, "cannot scan row")
		}

		if err := item.UpdateCloud(); err != nil {
			return 0, emperror.Wrapf(err, "error creating/updating item %v.%v", group.Id, item.Key)
		}
		counter++
	}
	return counter, nil
}

func (group *Group) syncItems(trashed bool) (int64, int64, error) {
	group.zot.logger.Infof("Syncing items of group #%v", group.Id)
	var counter int64

	objectList, lastModifiedVersion, err := group.GetItemsVersionCloud(group.ItemVersion, trashed)
	if err != nil {
		return counter, 0, emperror.Wrapf(err, "cannot get collection versions")
	}
	itemsUpdate := []string{}
	for itemid, version := range *objectList {
		oldversion, sync, err := group.GetItemVersionLocal(itemid, "")
		if err != nil {
			return counter, 0, emperror.Wrapf(err, "cannot get version of item %v from database: %v", itemid, err)
		}
		if sync != SyncStatus_Synced && sync != SyncStatus_Incomplete {
			return counter, 0, errors.New(fmt.Sprintf("item %v not synced. please handle conflict", itemid))
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
		items, err := group.GetItemsCloud(part)
		if err != nil {
			return counter, 0, emperror.Wrapf(err, "cannot get items")
		}
		group.zot.logger.Infof("%v items", len(*items))
		for _, item := range *items {
			group.zot.logger.Infof("Item %v of %v", counter, numItems)
			item.Status = SyncStatus_Synced
			item.Trashed = trashed
			if err := item.UpdateLocal(); err != nil {
				return counter, 0, emperror.Wrapf(err, "cannot update items")
			}
			counter++
		}
	}
	group.zot.logger.Infof("Syncing items of group #%v done. %v items changed", group.Id, counter)
	return counter, lastModifiedVersion, nil
}

func (group *Group) syncItemsGitlab() error {
	synctime := time.Now()
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5, gitlab"+
		" FROM %s.items"+
		" WHERE library=$1 AND (gitlab < modified OR gitlab is null)", group.zot.dbSchema)
	rows, err := group.zot.db.Query(sqlstr, group.Id)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, group.Id)
	}

	result := []Item{}
	for rows.Next() {
		item, err := group.itemFromRow(rows)
		if err != nil {
			rows.Close()
			return emperror.Wrapf(err, "cannot scan row")
		}
		result = append(result, *item)
	}
	rows.Close()

	num := int64(len(result))
	slices := num / 20
	if num%20 > 0 {
		slices++
	}
	for i := int64(0); i < slices; i++ {
		start := i * 20
		end := i*20 + 20
		if num < end {
			end = num
		}
		parts := result[start:end]
		gbranch := "master"
		gcommit := fmt.Sprintf("#%v machine sync  at %v", synctime.String())
		gaction := []*gitlab.CommitAction{}
		for _, item := range parts {
			// new and deleted -> we will not upload
			if item.Gitlab == nil && item.Deleted {
				continue
			}

			// check for attachment and upload if necessary
			itemType, err := item.GetType()
			if err != nil {
				return emperror.Wrapf(err, "cannot get item type of %v.%v", group.Id, item.Key)
			}
			if itemType == "attachment" && item.Data.MD5 != "" && !item.Deleted {
				folder, err := group.GetAttachmentFolder()
				if err != nil {
					return emperror.Wrapf(err, "cannot get attachment folder")
				}
				filename := fmt.Sprintf("%s/%s", folder, item.Key)
				content, err := ioutil.ReadFile(filename)
				if err != nil {
					return emperror.Wrapf(err, "cannot read %v", filename)
				}
				if err := item.UploadAttachmentGitlab(content); err != nil {
					return emperror.Wrapf(err, "cannot upload filedata for %v.%v", group.Id, item.Key)
				}
			}
			data, err := json.Marshal(item.Data)
			if err != nil {
				return emperror.Wrapf(err, "cannot marshall data %v", item.Data)
			}
			var prettyJSON bytes.Buffer
			err = json.Indent(&prettyJSON, data, "", "\t")
			if err != nil {
				return emperror.Wrapf(err, "cannot pretty json")
			}

			action := gitlab.CommitAction{
				Content:prettyJSON.String(),
			}
			if item.Gitlab == nil {
				action.Action = "create"
			} else if item.Deleted {
				action.Action = "delete"
			} else {
				action.Action = "update"
			}

			var fname string
			if string(item.Data.ParentItem) != "" {
				fname = fmt.Sprintf("%v/items/%v/%v.json", item.group.Id, string(item.Data.ParentItem), item.Key)
			} else {
				fname = fmt.Sprintf("%v/items/%v.json", item.group.Id, item.Key)
			}
			action.FilePath = fname
			gaction = append(gaction, &action)
		}
		opt := gitlab.CreateCommitOptions{
			Branch:        &gbranch,
			CommitMessage: &gcommit,
			StartBranch:   nil,
			Actions:       gaction,
			AuthorEmail:   nil,
			AuthorName:    nil,
		}
		group.zot.logger.Infof("committing %v items of %v to gitlab [%v:%v]", len(gaction), num, start, end)
		_, _, err := group.zot.git.Commits.CreateCommit(group.zot.gitProject.ID, &opt)
		if err != nil {
			return emperror.Wrapf(err, "cannot commit")
		}
		sqlstr := fmt.Sprintf("UPDATE %s.items SET gitlab=$1 WHERE library=$2 AND key=$3", group.zot.dbSchema)
		for _, item := range parts {
			t := sql.NullTime{
				Time:  synctime,
				Valid: !item.Deleted,
			}
			params := []interface{}{
				t,
				group.Id,
				item.Key,
			}
			if _, err := group.zot.db.Exec(sqlstr, params...); err != nil {
				return emperror.Wrapf(err, "cannot update gitlab sync time for %v.%v", group.Id, item.Key)
			}
		}
	}
	return nil
}

func (group *Group) SyncItems() (int64, int64, error) {
	var counter int64
	var err error
	var lastModifiedVersion int64

	if group.CanUpload() {
		counter, err = group.syncModifiedItems()
		if err != nil {
			return 0, 0, err
		}
	}

	if group.CanDownload() {
		var num2 int64
		num2, lastModifiedVersion, err = group.syncItems(true)
		if err != nil {
			return 0, 0, err
		}
		counter += num2
		var h int64
		var num3 int64
		num3, h, err = group.syncItems(false)
		if err != nil {
			return 0, 0, err
		}
		counter += num3
		if h > lastModifiedVersion {
			lastModifiedVersion = h
		}
	}
	if counter > 0 {
		group.zot.logger.Infof("refreshing materialized view item_type_hier")
		sqlstr := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s.item_type_hier WITH DATA", group.zot.dbSchema)
		_, err := group.zot.db.Exec(sqlstr)
		if err != nil {
			return counter, 0, emperror.Wrapf(err, "cannot refresh materialized view item_type_hier - %v", sqlstr)
		}
	}

	err = group.syncItemsGitlab()
	if err != nil {
		group.zot.logger.Errorf("cannot sync: %v", err)
	}

	return counter, lastModifiedVersion, nil
}

func (group *Group) GetItemByKeyLocal(key string) (*Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5, gitlab FROM %s.items"+
		" WHERE library=$1 AND key=$2", group.zot.dbSchema)
	params := []interface{}{
		group.Id,
		key,
	}

	item, err := group.itemFromRow(group.zot.db.QueryRow(sqlstr, params...))
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return item, nil
}

func (group *Group) GetItemByOldidLocal(oldid string) (*Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND oldid=$2", group.zot.dbSchema)
	params := []interface{}{
		group.Id,
		oldid,
	}
	item, err := group.itemFromRow(group.zot.db.QueryRow(sqlstr, params...))
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return item, nil
}

func (group *Group) TryDeleteItemLocal(key string, lastModifiedVersion int64) error {
	item, err := group.GetItemByKeyLocal(key)
	if err != nil {
		return emperror.Wrapf(err, "cannot get item %v", key)
	}
	// no item no deletion
	if item == nil {
		return nil
	}

	// already deleted
	if item.Deleted {
		return nil
	}

	if item.Status == SyncStatus_Synced {
		// all fine. delete item
		item.Deleted = true
	} else if group.direction == SyncDirection_ToLocal || group.direction == SyncDirection_BothCloud {
		// cloud leads --> delete
		item.Deleted = true
		item.Status = SyncStatus_Synced
	} else {
		// local leads sync back to cloud
		item.Version = lastModifiedVersion
		item.Status = SyncStatus_Synced
	}
	if err := item.UpdateLocal(); err != nil {
		return emperror.Wrapf(err, "cannot update collection %v", key)
	}
	return nil
}

func (group *Group) itemFromRow(rowss interface{}) (*Item, error) {

	item := Item{}
	var datastr sql.NullString
	var sync string
	var md5str sql.NullString
	var gitlab sql.NullTime
	switch rowss.(type) {
	case *sql.Row:
		row := rowss.(*sql.Row)
		if err := row.Scan(&item.Key, &item.Version, &datastr, &item.Trashed, &item.Deleted, &sync, &md5str, &gitlab); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, emperror.Wrapf(err, "cannot scan row")
		}
	case *sql.Rows:
		rows := rowss.(*sql.Rows)
		if err := rows.Scan(&item.Key, &item.Version, &datastr, &item.Trashed, &item.Deleted, &sync, &md5str, &gitlab); err != nil {
			return nil, emperror.Wrapf(err, "cannot scan row")
		}
	default:
		return nil, errors.New(fmt.Sprintf("unknown row type: %v", reflect.TypeOf(rowss).String()))
	}
	if md5str.Valid {
		item.MD5 = md5str.String
	}
	if gitlab.Valid {
		item.Gitlab = &gitlab.Time
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
