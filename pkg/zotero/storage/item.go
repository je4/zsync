package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

var errEmptyItem = errors.New("item has no data")

func (s *Storage) itemFromRow(groupId int64, rowss any) (*model.Item, error) {
	item := &model.Item{}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var md5str sql.NullString
	var gitlab sql.NullTime
	switch r := rowss.(type) {
	case *sql.Row:
		if err := r.Scan(&(item.Key), &(item.Version), &datastr, &metastr, &(item.Trashed), &(item.Deleted), &sync, &md5str, &gitlab); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, errors.Wrapf(err, "cannot scan row")
		}
	case *sql.Rows:
		if err := r.Scan(&(item.Key), &(item.Version), &datastr, &metastr, &(item.Trashed), &(item.Deleted), &sync, &md5str, &gitlab); err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
	default:
		return nil, errors.Errorf("unknown row type: %v", reflect.TypeOf(rowss).String())
	}
	if md5str.Valid {
		item.MD5 = md5str.String
	}
	if gitlab.Valid {
		item.Gitlab = &gitlab.Time
	}
	item.Status = model.SyncStatusId[sync]
	item.Library.Id = groupId
	item.Library.Type = "group"
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &(item.Data)); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal data %s", datastr.String)
		}
		if item.Data.Collections == nil {
			item.Data.Collections = []string{}
		}
	} else {
		return nil, errors.Wrapf(errEmptyItem, "item has no data %v.%v", groupId, item.Key)
	}
	if metastr.Valid {
		if err := json.Unmarshal([]byte(metastr.String), &(item.Meta)); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal meta %s", metastr.String)
		}
	} else {
		item.Meta = model.ItemMeta{
			CreatedByUser:  model.User{},
			CreatorSummary: "",
			ParsedDate:     "",
			NumChildren:    0,
		}
	}
	item.Data.ItemDataBase.Key = item.Key
	item.Data.ItemDataBase.Version = item.Version

	return item, nil
}

func (s *Storage) CreateItem(groupId int64, itemData *model.ItemGeneric, itemMeta *model.ItemMeta, oldId string) (*model.Item, error) {
	if itemData.Key == "" {
		itemData.Key = model.CreateKey()
	}
	if itemMeta == nil {
		itemMeta = &model.ItemMeta{}
	}
	item := &model.Item{
		Key:     itemData.Key,
		Version: 0,
		Library: model.Library{
			Id:   groupId,
			Type: "group",
		},
		Meta:   *itemMeta,
		Data:   *itemData,
		OldId:  oldId,
		Status: model.SyncStatus_New,
	}
	jsonstr, err := json.Marshal(itemData)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot marshal item data %v", itemData)
	}
	oid := sql.NullString{
		String: oldId,
		Valid:  true,
	}
	if oldId == "" {
		oid.Valid = false
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, sync, data, oldid) VALUES( $1, $2, $3, $4, $5, $6)", s.dbSchema)
	params := []any{
		item.Key,
		0,
		item.Library.Id,
		"new",
		string(jsonstr),
		oid,
	}
	_, err = s.db.Exec(sqlstr, params...)
	if IsUniqueViolation(err, "items_oldid_constraint") {
		item2, err := s.GetItemByOldid(groupId, oldId)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot load item %v", oldId)
		}
		item.Key = item2.Key
		item.Data.Key = item.Key
		item.Version = item2.Version
		item.Status = model.SyncStatus_Modified
		err = s.UpdateItem(groupId, item)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot update item %v", oldId)
		}
	} else if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return item, nil
}

func (s *Storage) CreateEmptyItem(groupId int64, itemId string, oldId string) error {
	oid := sql.NullString{
		String: oldId,
		Valid:  true,
	}
	if oldId == "" {
		oid.Valid = false
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.items (key, version, library, sync, oldid) VALUES( $1, 0, $2, $3, $4)", s.dbSchema)
	params := []any{
		itemId,
		groupId,
		"incomplete",
		oid,
	}
	_, err := s.db.Exec(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) GetItemVersion(groupId int64, itemId string, oldId string) (int64, model.SyncStatus, error) {
	sqlstr := fmt.Sprintf("SELECT version, sync FROM %s.items WHERE library=$1 AND key=$2", s.dbSchema)
	params := []any{
		groupId,
		itemId,
	}
	var version int64
	var syncstr string
	var sync model.SyncStatus
	err := s.db.QueryRow(sqlstr, params...).Scan(&version, &syncstr)
	switch {
	case err == sql.ErrNoRows:
		if err := s.CreateEmptyItem(groupId, itemId, oldId); err != nil {
			return 0, model.SyncStatus_New, errors.Wrapf(err, "cannot create new item")
		}
		version = 0
		sync = model.SyncStatus_New
	case err != nil:
		return 0, model.SyncStatus_New, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	case err == nil:
		sync = model.SyncStatusId[syncstr]
	}
	return version, sync, nil
}

func (s *Storage) GetItems(groupId int64, objectKeys []string) (*[]model.Item, error) {
	if len(objectKeys) == 0 {
		return &[]model.Item{}, nil
	}
	params := []any{
		groupId,
	}
	pstr := []string{}
	for _, val := range objectKeys {
		params = append(params, val)
		pstr = append(pstr, fmt.Sprintf("$%v", len(params)))
	}
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND key IN (%s)", s.dbSchema, strings.Join(pstr, ","))
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return &[]model.Item{}, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()

	result := []model.Item{}
	counter := 0
	for rows.Next() {
		counter++
		item, err := s.itemFromRow(groupId, rows)
		if err != nil {
			if errors.Is(err, errEmptyItem) {
				if s.Logger != nil {
					s.Logger.Warn().Err(err).Msgf("item #%v is empty. skipping", counter)
				}
				continue
			}
			return nil, errors.Wrapf(err, "cannot scan row")
		}
		result = append(result, *item)
	}
	return &result, nil
}

func (s *Storage) GetItemsVersion(groupId int64, sinceVersion int64, trashed bool) (*map[string]int64, int64, error) {
	sqlstr := fmt.Sprintf("SELECT key, version FROM %s.items WHERE library=$1 AND version > $2 AND trashed=$3", s.dbSchema)
	params := []any{
		groupId,
		sinceVersion,
		trashed,
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()

	objects := map[string]int64{}
	var lastModifiedVersion int64 = sinceVersion
	for rows.Next() {
		var key string
		var version int64
		if err := rows.Scan(&key, &version); err != nil {
			return nil, 0, errors.Wrap(err, "cannot scan row")
		}
		objects[key] = version
		if version > lastModifiedVersion {
			lastModifiedVersion = version
		}
	}
	return &objects, lastModifiedVersion, nil
}

func (s *Storage) GetItemByKey(groupId int64, key string) (*model.Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM %s.items"+
		" WHERE library=$1 AND key=$2", s.dbSchema)
	params := []any{
		groupId,
		key,
	}

	item, err := s.itemFromRow(groupId, s.db.QueryRow(sqlstr, params...))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return item, nil
}

func (s *Storage) GetItemByOldid(groupId int64, oldid string) (*model.Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab FROM %s.items WHERE library=$1 AND oldid=$2", s.dbSchema)
	params := []any{
		groupId,
		oldid,
	}
	item, err := s.itemFromRow(groupId, s.db.QueryRow(sqlstr, params...))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return item, nil
}

func (s *Storage) UpdateItem(groupId int64, item *model.Item) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("updating item [#%s]", item.Key)
	}
	var md5val sql.NullString
	if item.MD5 == "" {
		if item.Data.ItemType == "attachment" {
			md5val.String = item.Data.MD5
			md5val.Valid = true
		}
	} else {
		md5val.String = item.MD5
		md5val.Valid = true
	}
	md5val.Valid = md5val.String != ""

	data, err := json.Marshal(item.Data)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal data %v", item.Data)
	}
	meta, err := json.Marshal(item.Meta)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal meta %v", item.Meta)
	}
	var sqlstr string
	var params []any
	if item.Version > 0 {
		sqlstr = fmt.Sprintf("UPDATE %s.items SET version=$1, data=$2, meta=$3, trashed=$4, deleted=$5, sync=$6, md5=$7, modified=NOW() "+
			"WHERE library=$8 AND key=$9", s.dbSchema)
		params = []any{
			item.Version,
			string(data),
			string(meta),
			item.Trashed,
			item.Deleted,
			model.SyncStatusString[model.SyncStatus_Modified],
			md5val,
			groupId,
			item.Key,
		}
	} else {
		sqlstr = fmt.Sprintf("UPDATE %s.items SET data=$1, meta=$2, trashed=$3, deleted=$4, sync=$5, md5=$6, modified=NOW() "+
			"WHERE library=$7 AND key=$8", s.dbSchema)
		params = []any{
			string(data),
			string(meta),
			item.Trashed,
			item.Deleted,
			model.SyncStatusString[item.Status],
			md5val,
			groupId,
			item.Key,
		}
	}
	_, err = s.db.Exec(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) DeleteItem(groupId int64, key string) error {
	sqlstr := fmt.Sprintf("UPDATE %s.items SET deleted=true, sync=$1, modified=NOW() WHERE key=$2 AND library=$3", s.dbSchema)
	params := []any{
		model.SyncStatusString[model.SyncStatus_Modified],
		key,
		groupId,
	}
	if _, err := s.db.Exec(sqlstr, params...); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) GetChildren(groupId int64, key string) (*[]model.Item, error) {
	if s.Logger != nil {
		s.Logger.Info().Msgf("get children of item [#%s]", key)
	}
	sqlstr := fmt.Sprintf("SELECT i.key, i.version, i.data, i.meta, i.trashed, i.deleted, i.sync, i.md5, i.gitlab"+
		" FROM %s.items i, %s.item_type_hier ith"+
		" WHERE i.trashed=false AND i.deleted=false AND i.key=ith.key AND i.library=ith.library AND i.library=$1 AND ith.parent=$2", s.dbSchema, s.dbSchema)
	params := []any{
		groupId,
		key,
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		if err == sql.ErrNoRows {
			return &[]model.Item{}, nil
		}
		return &[]model.Item{}, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()
	items := []model.Item{}
	for rows.Next() {
		i, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return &[]model.Item{}, errors.Wrapf(err, "cannot scan result row")
		}
		items = append(items, *i)
	}

	return &items, nil
}

func (s *Storage) DeleteItemRecursive(groupId int64, key string) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("recursively deleting item [#%s]", key)
	}
	children, err := s.GetChildren(groupId, key)
	if err != nil {
		return errors.Wrapf(err, "cannot get children of #%v", key)
	}
	for _, c := range *children {
		if err := s.DeleteItemRecursive(groupId, c.Key); err != nil {
			return errors.Wrapf(err, "cannot delete child #%v of #%v", c.Key, key)
		}
	}
	return s.DeleteItem(groupId, key)
}

func (s *Storage) IterateItems(groupId int64, after *time.Time, f func(item *model.Item) error) error {
	sqlstr0 := fmt.Sprintf("SELECT COUNT(*) "+
		"FROM %s.items WHERE library=$1 AND deleted=false", s.dbSchema)
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab "+
		"FROM %s.items WHERE library=$1 AND deleted=false", s.dbSchema)
	params := []any{
		groupId,
	}
	if after != nil {
		sqlstr0 += " AND (modified > TO_TIMESTAMP($2, 'YYYY-MM-DD HH24:MI:SS'))"
		sqlstr += " AND (modified > TO_TIMESTAMP($2, 'YYYY-MM-DD HH24:MI:SS'))"
		params = append(params, after.Format("2006-01-02 15:04:05"))
	}
	var num int64
	if err := s.db.QueryRow(sqlstr0, params...).Scan(&num); err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr0)
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("%v items found", num)
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get item")
		}
		if err := f(item); err != nil {
			return errors.Wrapf(err, "error in callback for %v", item.Key)
		}
	}
	return nil
}

func (s *Storage) IterateItemsAll(groupId int64, after *time.Time, f func(item *model.Item) error) error {
	sqlstr0 := fmt.Sprintf("SELECT COUNT(*) "+
		"FROM %s.items WHERE library=$1", s.dbSchema)
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab "+
		"FROM %s.items WHERE library=$1", s.dbSchema)
	params := []any{
		groupId,
	}
	if after != nil {
		sqlstr0 += " AND (modified > TO_TIMESTAMP($2, 'YYYY-MM-DD HH24:MI:SS'))"
		sqlstr += " AND (modified > TO_TIMESTAMP($2, 'YYYY-MM-DD HH24:MI:SS'))"
		params = append(params, after.Format("2006-01-02 15:04:05"))
	}
	var num int64
	if err := s.db.QueryRow(sqlstr0, params...).Scan(&num); err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr0)
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("%v items found", num)
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get item")
		}
		if err := f(item); err != nil {
			return errors.Wrapf(err, "error in callback for %v", item.Key)
		}
	}
	return nil
}

func (s *Storage) GetModifiedItems(groupId int64) ([]*model.Item, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, trashed, deleted, sync, md5, gitlab"+
		" FROM %s.items"+
		" WHERE library=$1 AND (sync=$2 or sync=$3)", s.dbSchema)
	params := []any{
		groupId,
		"new",
		"modified",
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()

	items := []*model.Item{}
	for rows.Next() {
		item, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Storage) RefreshItemTypeHier() error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("refreshing materialized view item_type_hier")
	}
	sqlstr := fmt.Sprintf("SELECT %s.refresh_item_type_hier()", s.dbSchema)
	if _, err := s.db.Exec(sqlstr); err != nil {
		return errors.Wrapf(err, "cannot refresh materialized view item_type_hier - %v", sqlstr)
	}
	return nil
}

func (s *Storage) UpdateItemsGitlabTimestamp(groupId int64, now time.Time, gitlab *time.Time) error {
	sqlstr := fmt.Sprintf("UPDATE %s.items SET gitlab=TO_TIMESTAMP($1, 'YYYY-MM-DD HH24:MI:SS') "+
		"WHERE library=$2 AND (TO_TIMESTAMP($3, 'YYYY-MM-DD HH24:MI:SS') > gitlab OR gitlab IS NULL)", s.dbSchema)
	params := []any{
		now.Format("2006-01-02 15:04:05"),
		groupId,
		now.Format("2006-01-02 15:04:05"),
	}
	if gitlab != nil {
		sqlstr += " AND (gitlab >= TO_TIMESTAMP($4, 'YYYY-MM-DD HH24:MI:SS') OR gitlab IS NULL)"
		params = append(params, gitlab.Format("2006-01-02 15:04:05"))
	}
	if _, err := s.db.Exec(sqlstr, params...); err != nil {
		return errors.Wrapf(err, "cannot execute %v - %v", sqlstr, params)
	}
	return nil
}
