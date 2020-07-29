package zotero

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/xanzy/go-gitlab"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/filesystem"
	"gopkg.in/resty.v1"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func (group *Group) DeleteItemLocal(key string) error {
	sqlstr := fmt.Sprintf("UPDATE %s.items SET deleted=true WHERE key=$1 AND library=$2", group.Zot.dbSchema)

	params := []interface{}{
		key,
		group.Id,
	}
	if _, err := group.Zot.db.Exec(sqlstr, params...); err != nil {
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
		Group:   group,
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
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, sync, data, oldid) VALUES( $1, $2, $3, $4, $5, $6)", group.Zot.dbSchema)
	params := []interface{}{
		item.Key,
		0,
		item.Library.Id,
		"new",
		string(jsonstr),
		oid,
	}
	_, err = group.Zot.db.Exec(sqlstr, params...)
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

	if item.Group.Zot.git != nil {
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
		fileinfo, _, err := item.Group.Zot.git.RepositoryFiles.CreateFile(item.Group.Zot.gitProject.ID, fmt.Sprintf("%v/%v", item.Group.Id, item.Key), &gopt)
		if err != nil {
			return nil, emperror.Wrapf(err, "upload to gitlab failed")
		}
		item.Group.Zot.logger.Debugf("upload to gitlab done: %v", fileinfo.String())
	}
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
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, sync, oldid) VALUES( $1, 0, $2, $3, $4)", group.Zot.dbSchema)
	params := []interface{}{
		itemId,
		group.Id,
		"incomplete",
		oid,
	}
	_, err := group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (group *Group) GetItemVersionLocal(itemId string, oldId string) (int64, SyncStatus, error) {
	sqlstr := fmt.Sprintf("SELECT version, sync FROM %s.items WHERE library=$1 AND key=$2", group.Zot.dbSchema)
	params := []interface{}{
		group.Id,
		itemId,
	}
	var version int64
	var syncstr string
	var sync SyncStatus
	err := group.Zot.db.QueryRow(sqlstr, params...).Scan(&version, &syncstr)
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
		group.Zot.logger.Infof("rest call: %s [%v, %v]", endpoint, start, limit)
		call := group.Zot.client.R().
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
			if !group.Zot.CheckRetry(resp.Header()) {
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
		h, _ := strconv.ParseInt(resp.RawResponse.Header.Get("Last-IsModified-Version"), 10, 64)
		if h > lastModifiedVersion {
			lastModifiedVersion = h
		}
		group.Zot.CheckBackoff(resp.Header())
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
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND key IN (%s)", group.Zot.dbSchema, strings.Join(pstr, ","))
	rows, err := group.Zot.db.Query(sqlstr, params...)
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

func (group *Group) IterateItemsAllLocal(after *time.Time, f func(item *Item) error) error {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab " +
		"FROM %s.items WHERE library=$1", group.Zot.dbSchema)
	params := []interface{}{
		group.Id,
	}
	if after != nil {
		sqlstr += " AND (gitlab IS NULL OR gitlab > TO_TIMESTAMP($2, 'YYYY-MM-DD HH24:MI:SS'))"
		params = append(params, after.Format("2006-01-02 15:04:05"))
	}
	rows, err := group.Zot.db.Query(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := group.itemFromRow(rows)
		if err != nil {
			return emperror.Wrapf(err, "cannot scan row")
		}
		if err := f(item); err != nil {
			return err
		}
	}
	return nil
}

func (group *Group) IterateCollectionsAllLocal(after *time.Time, f func(coll *Collection) error) error {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, deleted, sync, gitlab"+
		" FROM %s.collections"+
		" WHERE library=$1", group.Zot.dbSchema)

	params := []interface{}{
		group.Id,
	}
	if after != nil {
		sqlstr += " AND (gitlab IS NULL OR gitlab > TO_TIMESTAMP($2, 'YYYY-MM-DD HH24:MI:SS'))"
		params = append(params, after.Format("2006-01-02 15:04:05"))
	}
	rows, err := group.Zot.db.Query(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		coll, err := group.collectionFromRow(rows)
		if err != nil {
			return emperror.Wrapf(err, "cannot scan row")
		}
		if err := f(coll); err != nil {
			return err
		}
	}
	return nil
}

func (group *Group) GetItemsCloud(objectKeys []string) (*[]Item, error) {
	if len(objectKeys) == 0 {
		return &[]Item{}, nil
	}
	if len(objectKeys) > 50 {
		return nil, errors.New("too much objectKeys (max. 50)")
	}

	endpoint := fmt.Sprintf("/groups/%v/items", group.Id)
	group.Zot.logger.Infof("rest call: %s", endpoint)

	call := group.Zot.client.R().
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
		if !group.Zot.CheckRetry(resp.Header()) {
			break
		}
	}
	group.Zot.logger.Debugf("status: #%v ", resp.StatusCode())
	rawBody := resp.Body()
	items := []Item{}
	if err := json.Unmarshal(rawBody, &items); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	group.Zot.CheckBackoff(resp.Header())
	result := []Item{}
	for _, item := range items {
		item.Group = group
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
		result = append(result, item)
	}
	return &result, nil
}

func (group *Group) syncModifiedItems(lastModifiedVersion int64) (int64, error) {
	var counter int64
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab"+
		" FROM %s.items"+
		" WHERE library=$1 AND (sync=$2 or sync=$3)", group.Zot.dbSchema)
	params := []interface{}{
		group.Id,
		"new",
		"modified",
	}
	rows, err := group.Zot.db.Query(sqlstr, params...)
	if err != nil {
		return 0, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := group.itemFromRow(rows)
		if err != nil {
			return 0, emperror.Wrapf(err, "cannot scan row")
		}

		if err := item.UpdateCloud(&lastModifiedVersion); err != nil {
			group.Zot.logger.Errorf("error creating/updating item %v.%v: %v", group.Id, item.Key, err)
			//return 0, emperror.Wrapf(err, "error creating/updating item %v.%v", Group.Id, item.Key)
		}
		counter++
	}
	return counter, nil
}

func (group *Group) syncItems(trashed bool) (int64, int64, error) {
	group.Zot.logger.Infof("Syncing items of Group #%v", group.Id)
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
			group.Zot.logger.Errorf("item %v not synced. please handle conflict", itemid)
			continue
			//return counter, lastModifiedVersion, errors.New(fmt.Sprintf("item %v not synced. please handle conflict", itemid))
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
		group.Zot.logger.Infof("%v items", len(*items))
		for _, item := range *items {
			group.Zot.logger.Infof("Item %v of %v", counter, numItems)
			item.Status = SyncStatus_Synced
			item.Trashed = trashed
			if err := item.UpdateLocal(); err != nil {
				group.Zot.logger.Errorf("cannot update item: %v", err)
				//return counter, 0, emperror.Wrapf(err, "cannot update items")
			}
			counter++
		}
	}
	group.Zot.logger.Infof("Syncing items of Group #%v done. %v items changed", group.Id, counter)
	return counter, lastModifiedVersion, nil
}

func (group *Group) syncItemsGitlab() error {
	if group.Zot.git == nil {
		return nil
	}
	synctime := time.Now()
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab"+
		" FROM %s.items"+
		" WHERE library=$1 AND (gitlab < modified OR gitlab is null)", group.Zot.dbSchema)
	rows, err := group.Zot.db.Query(sqlstr, group.Id)
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
		if (item.Deleted || item.Trashed) && item.Gitlab == nil {
			continue
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
		gaction := []*gitlab.CommitAction{}
		var creations int64
		var deletions int64
		var updates int64
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
//			var event gitlab.EventTypeValue
			if itemType == "attachment" && item.Data.MD5 != "" && !item.Deleted {
				folder, err := group.GetFolder()
				if err != nil {
					return emperror.Wrapf(err, "cannot get attachment folder")
				}
				content, err := group.Zot.fs.FileGet(folder, item.Key, filesystem.FileGetOptions{})
				if err != nil {
					return emperror.Wrapf(err, "cannot read %v/&v", folder, item.Key)
				}
				_, err = item.UploadAttachmentGitlab(content)
				if err != nil {
					return emperror.Wrapf(err, "cannot upload filedata for %v.%v", group.Id, item.Key)
				}
			}
			ig := ItemGitlab{
				LibraryId: item.Group.ItemVersion,
				Key:       item.Key,
				Data:      item.Data,
				Meta:      item.Meta,
			}

			data, err := json.Marshal(ig)
			if err != nil {
				return emperror.Wrapf(err, "cannot marshall data %v", item.Data)
			}
			var prettyJSON bytes.Buffer
			err = json.Indent(&prettyJSON, data, "", "\t")
			if err != nil {
				return emperror.Wrapf(err, "cannot pretty json")
			}
			action := gitlab.CommitAction{
				Content: prettyJSON.String(),
			}
			/*
			switch event {
			case gitlab.CreatedEventType:
				action.Action = "create"
				creations++
			case gitlab.DestroyedEventType:
				action.Action = "delete"
				deletions++
			case gitlab.UpdatedEventType:
				action.Action = "update"
				updates++
			}
			 */

			if item.Gitlab == nil {
				action.Action = "create"
				creations++
			} else if item.Deleted || item.Trashed {
				action.Action = "delete"
				deletions++
			} else {
				action.Action = "update"
				updates++
			}

			var fname string
			if string(item.Data.ParentItem) != "" {
				fname = fmt.Sprintf("%v/items/%v/%v.json", item.Group.Id, string(item.Data.ParentItem), item.Key)
			} else {
				fname = fmt.Sprintf("%v/items/%v.json", item.Group.Id, item.Key)
			}
			action.FilePath = fname

			found, err := group.Zot.gitlabCheck(fname, "master")
			if err != nil {
				return emperror.Wrapf(err, "cannot check gitlab for %v", fname)
			}

			if !found {
				switch action.Action {
				case "delete":
					action.Action = ""
				case "update":
					action.Action = "create"
				}
			} else {
				switch action.Action {
				case "create":
					action.Action = "update"
				}
			}

			if action.Action != "" {
				gaction = append(gaction, &action)
			}
		}
		gcommit := fmt.Sprintf("#%v/%v machine sync creation:%v / deletion:%v / update:%v  at %v",
			i+1, slices, creations, deletions, updates, synctime.String())
		opt := gitlab.CreateCommitOptions{
			Branch:        &gbranch,
			CommitMessage: &gcommit,
			StartBranch:   nil,
			Actions:       gaction,
			AuthorEmail:   nil,
			AuthorName:    nil,
		}
		group.Zot.logger.Infof("committing item %v to %v of %v to gitlab", start, end, num)
		_, _, err := group.Zot.git.Commits.CreateCommit(group.Zot.gitProject.ID, &opt)
		if err != nil {
			fps := []string{}
			for _, action := range opt.Actions {
				fps = append(fps, action.FilePath)
			}
			group.Zot.logger.Errorf("cannot commit files %v: %v", fps, err)
			//return emperror.Wrapf(err, "cannot commit")
		} else {
			sqlstr := fmt.Sprintf("UPDATE %s.items SET gitlab=$1 WHERE library=$2 AND key=$3", group.Zot.dbSchema)
			for _, item := range parts {
				t := sql.NullTime{
					Time:  synctime,
					Valid: !(item.Deleted || item.Trashed),
				}
				params := []interface{}{
					t,
					group.Id,
					item.Key,
				}
				if _, err := group.Zot.db.Exec(sqlstr, params...); err != nil {
					return emperror.Wrapf(err, "cannot update gitlab sync time for %v.%v", group.Id, item.Key)
				}
			}
		}
	}
	return nil
}

func (group *Group) SyncItems() (int64, int64, error) {
	var counter int64
	var err error
	var lastModifiedVersion int64

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

	if group.CanUpload() {
		num4, err := group.syncModifiedItems(lastModifiedVersion)
		if err != nil {
			return 0, 0, err
		}
		counter += num4
	}

	if counter > 0 {
		group.Zot.logger.Infof("refreshing materialized view item_type_hier")
		sqlstr := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s.item_type_hier WITH DATA", group.Zot.dbSchema)
		_, err := group.Zot.db.Exec(sqlstr)
		if err != nil {
			return counter, 0, emperror.Wrapf(err, "cannot refresh materialized view item_type_hier - %v", sqlstr)
		}
	}

	err = group.syncItemsGitlab()
	if err != nil {
		group.Zot.logger.Errorf("cannot sync: %v", err)
	}

	return counter, lastModifiedVersion, nil
}

func (group *Group) GetItemByKeyLocal(key string) (*Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM %s.items"+
		" WHERE library=$1 AND key=$2", group.Zot.dbSchema)
	params := []interface{}{
		group.Id,
		key,
	}

	item, err := group.itemFromRow(group.Zot.db.QueryRow(sqlstr, params...))
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return item, nil
}

func (group *Group) GetItemByOldidLocal(oldid string) (*Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND oldid=$2", group.Zot.dbSchema)
	params := []interface{}{
		group.Id,
		oldid,
	}
	item, err := group.itemFromRow(group.Zot.db.QueryRow(sqlstr, params...))
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

	item := &Item{}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var md5str sql.NullString
	var gitlab sql.NullTime
	switch rowss.(type) {
	case *sql.Row:
		row := rowss.(*sql.Row)
		if err := row.Scan(&(item.Key), &(item.Version), &datastr, &metastr, &(item.Trashed), &(item.Deleted), &sync, &md5str, &gitlab); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, emperror.Wrapf(err, "cannot scan row")
		}
	case *sql.Rows:
		rows := rowss.(*sql.Rows)
		if err := rows.Scan(&(item.Key), &(item.Version), &datastr, &metastr, &(item.Trashed), &(item.Deleted), &sync, &md5str, &gitlab); err != nil {
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
		if err := json.Unmarshal([]byte(datastr.String), &(item.Data)); err != nil {
			return nil, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
		}
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
	} else {
		return nil, errors.New(fmt.Sprintf("item has no data %v.%v", group.Id, item.Key))
	}
	if metastr.Valid {
		if err := json.Unmarshal([]byte(metastr.String), &(item.Meta)); err != nil {
			return nil, emperror.Wrapf(err, "cannot ummarshall meta %s", metastr.String)
		}
	} else {
		item.Meta = ItemMeta{
			CreatedByUser:  User{},
			CreatorSummary: "",
			ParsedDate:     "",
			NumChildren:    0,
		}
		//return nil, errors.New(fmt.Sprintf("item has no meta %v.%v", Group.Id, item.Key))
	}
	item.Group = group
	item.Data.ItemDataBase.Key = item.Key
	item.Data.ItemDataBase.Version = item.Version

	return item, nil
}
