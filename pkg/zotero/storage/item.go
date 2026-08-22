package storage

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"time"

	"emperror.dev/errors"
	"github.com/jackc/pgx/v5"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

var errEmptyItem = errors.New("item has no data")

func (s *Storage) itemFromRow(groupId int64, row pgx.Row) (*model.Item, error) {
	item := &model.Item{}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var md5str sql.NullString
	var gitlab sql.NullTime
	if err := row.Scan(&(item.Key), &(item.Version), &datastr, &metastr, &(item.Trashed), &(item.Deleted), &sync, &md5str, &gitlab); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "cannot scan row")
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
		if item.Deleted || item.Status == model.SyncStatus_Incomplete {
			return nil, nil
		}
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

func (s *Storage) CreateItem(ctx context.Context, groupId int64, itemData *model.ItemGeneric, itemMeta *model.ItemMeta, oldId string) (*model.Item, error) {
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
	params := pgx.NamedArgs{
		"key":     item.Key,
		"version": 0,
		"library": item.Library.Id,
		"sync":    "new",
		"data":    string(jsonstr),
		"oldid":   oid,
	}
	_, err = s.db.Exec(ctx, SQLInsertItem, params)
	if IsUniqueViolation(err, "items_oldid_constraint") {
		item2, err := s.GetItemByOldid(ctx, groupId, oldId)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot load item %v", oldId)
		}
		item.Key = item2.Key
		item.Data.Key = item.Key
		item.Version = item2.Version
		item.Status = model.SyncStatus_Modified
		err = s.UpdateItem(ctx, groupId, item)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot update item %v", oldId)
		}
	} else if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLInsertItem, params)
	}

	return item, nil
}

func (s *Storage) CreateEmptyItem(ctx context.Context, groupId int64, itemId string, oldId string) error {
	oid := sql.NullString{
		String: oldId,
		Valid:  true,
	}
	if oldId == "" {
		oid.Valid = false
	}
	params := pgx.NamedArgs{
		"key":     itemId,
		"library": groupId,
		"sync":    "incomplete",
		"oldid":   oid,
	}
	_, err := s.db.Exec(ctx, SQLInsertEmptyItem, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLInsertEmptyItem, params)
	}
	return nil
}

func (s *Storage) GetItemVersion(ctx context.Context, groupId int64, itemId string, oldId string) (int64, model.SyncStatus, error) {
	params := pgx.NamedArgs{
		"library": groupId,
		"key":     itemId,
	}
	var version int64
	var syncstr string
	var sync model.SyncStatus
	err := s.db.QueryRow(ctx, SQLGetItemVersion, params).Scan(&version, &syncstr)
	switch {
	case IsEmptyResult(err):
		if err := s.CreateEmptyItem(ctx, groupId, itemId, oldId); err != nil {
			return 0, model.SyncStatus_Incomplete, errors.Wrapf(err, "cannot create new item")
		}
		version = 0
		sync = model.SyncStatus_Incomplete
	case err != nil:
		return 0, model.SyncStatus_Incomplete, errors.Wrapf(err, "cannot execute %s: %v", SQLGetItemVersion, params)
	case err == nil:
		sync = model.SyncStatusId[syncstr]
	}
	return version, sync, nil
}

func (s *Storage) GetItemsByKey(ctx context.Context, groupId int64, objectKeys []string) ([]model.Item, error) {
	if len(objectKeys) == 0 {
		return []model.Item{}, nil
	}
	params := pgx.NamedArgs{
		"library": groupId,
		"keys":    objectKeys,
	}
	rows, err := s.db.Query(ctx, SQLGetItems, params)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLGetItems, params)
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
		if item == nil {
			continue
		}
		result = append(result, *item)
	}
	return result, nil
}

func (s *Storage) GetItemVersions(ctx context.Context, groupId int64, sinceVersion int64, trashed bool) (map[string]int64, int64, error) {
	params := pgx.NamedArgs{
		"library":      groupId,
		"sinceVersion": sinceVersion,
		"trashed":      trashed,
	}
	rows, err := s.db.Query(ctx, SQLGetItemVersions, params)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "cannot execute %s: %v", SQLGetItemVersions, params)
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
	return objects, lastModifiedVersion, nil
}

func (s *Storage) GetItemByKey(ctx context.Context, groupId int64, key string) (*model.Item, error) {
	params := pgx.NamedArgs{
		"library": groupId,
		"key":     key,
	}
	item, err := s.itemFromRow(groupId, s.db.QueryRow(ctx, SQLGetItemByKey, params))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLGetItemByKey, params)
	}

	return item, nil
}

func (s *Storage) GetItemByOldid(ctx context.Context, groupId int64, oldid string) (*model.Item, error) {
	params := pgx.NamedArgs{
		"library": groupId,
		"oldid":   oldid,
	}
	item, err := s.itemFromRow(groupId, s.db.QueryRow(ctx, SQLGetItemByOldid, params))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLGetItemByOldid, params)
	}

	return item, nil
}

func (s *Storage) UpdateItem(ctx context.Context, groupId int64, item *model.Item) error {
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
	params := pgx.NamedArgs{
		"data":    string(data),
		"meta":    string(meta),
		"trashed": item.Trashed,
		"deleted": item.Deleted,
		"sync":    model.SyncStatusString[item.Status],
		"md5":     md5val,
		"library": groupId,
		"key":     item.Key,
	}
	if item.Version > 0 {
		sqlstr = SQLUpdateItem
		params["version"] = item.Version
	} else {
		sqlstr = SQLUpdateItemVersion0
	}
	_, err = s.db.Exec(ctx, sqlstr, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) DeleteItem(ctx context.Context, groupId int64, key string) error {
	params := pgx.NamedArgs{
		"sync":    model.SyncStatusString[model.SyncStatus_Modified],
		"key":     key,
		"library": groupId,
	}
	if _, err := s.db.Exec(ctx, SQLDeleteItem, params); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", SQLDeleteItem, params)
	}
	return nil
}

// DeleteItems marks all given items as deleted in a single statement.
// It returns the number of affected rows.
func (s *Storage) DeleteItems(ctx context.Context, groupId int64, keys []string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	params := pgx.NamedArgs{
		"sync":    model.SyncStatusString[model.SyncStatus_Modified],
		"keys":    keys,
		"library": groupId,
	}
	tag, err := s.db.Exec(ctx, SQLDeleteItems, params)
	if err != nil {
		return 0, errors.Wrapf(err, "error executing %s: %v", SQLDeleteItems, params)
	}
	return tag.RowsAffected(), nil
}

func (s *Storage) GetChildren(ctx context.Context, groupId int64, key string) ([]model.Item, error) {
	if s.Logger != nil {
		s.Logger.Info().Msgf("get children of item [#%s]", key)
	}
	params := pgx.NamedArgs{
		"library": groupId,
		"parent":  key,
	}
	rows, err := s.db.Query(ctx, SQLGetChildren, params)
	if err != nil {
		if IsEmptyResult(err) {
			return []model.Item{}, nil
		}
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLGetChildren, params)
	}
	defer rows.Close()
	items := []model.Item{}
	for rows.Next() {
		i, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot scan result row")
		}
		if i == nil {
			continue
		}
		items = append(items, *i)
	}

	return items, nil
}

func (s *Storage) DeleteItemRecursive(ctx context.Context, groupId int64, key string) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("recursively deleting item [#%s]", key)
	}
	children, err := s.GetChildren(ctx, groupId, key)
	if err != nil {
		return errors.Wrapf(err, "cannot get children of #%v", key)
	}
	for _, c := range children {
		if err := s.DeleteItemRecursive(ctx, groupId, c.Key); err != nil {
			return errors.Wrapf(err, "cannot delete child #%v of #%v", c.Key, key)
		}
	}
	return s.DeleteItem(ctx, groupId, key)
}

func (s *Storage) IterateItems(ctx context.Context, groupId int64, after *time.Time, f func(item *model.Item) error) error {
	var sqlstr0, sqlstr string
	params := pgx.NamedArgs{
		"library": groupId,
	}
	if after != nil {
		sqlstr0 = SQLIterateItemsAfterCount
		sqlstr = SQLIterateItemsAfter
		params["after"] = after.Format("2006-01-02 15:04:05")
	} else {
		sqlstr0 = SQLIterateItemsCount
		sqlstr = SQLIterateItems
	}
	return s.iterateItems(ctx, groupId, sqlstr0, sqlstr, params, f)
}

func (s *Storage) IterateItemsAll(ctx context.Context, groupId int64, after *time.Time, f func(item *model.Item) error) error {
	var sqlstr0, sqlstr string
	params := pgx.NamedArgs{
		"library": groupId,
	}
	if after != nil {
		sqlstr0 = SQLIterateItemsAllAfterCount
		sqlstr = SQLIterateItemsAllAfter
		params["after"] = after.Format("2006-01-02 15:04:05")
	} else {
		sqlstr0 = SQLIterateItemsAllCount
		sqlstr = SQLIterateItemsAll
	}
	return s.iterateItems(ctx, groupId, sqlstr0, sqlstr, params, f)
}

func (s *Storage) iterateItems(ctx context.Context, groupId int64, countSql, sqlstr string, params pgx.NamedArgs, f func(item *model.Item) error) error {
	var num int64
	if err := s.db.QueryRow(ctx, countSql, params).Scan(&num); err != nil {
		return errors.Wrapf(err, "cannot execute %s", countSql)
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("%v items found", num)
	}
	rows, err := s.db.Query(ctx, sqlstr, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get item")
		}
		if item == nil {
			continue
		}
		if err := f(item); err != nil {
			return errors.Wrapf(err, "error in callback for %v", item.Key)
		}
	}
	return nil
}

func (s *Storage) GetModifiedItems(ctx context.Context, groupId int64) ([]*model.Item, error) {
	params := pgx.NamedArgs{
		"library":      groupId,
		"syncNew":      "new",
		"syncModified": "modified",
	}
	rows, err := s.db.Query(ctx, SQLGetModifiedItems, params)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLGetModifiedItems, params)
	}
	defer rows.Close()

	items := []*model.Item{}
	for rows.Next() {
		item, err := s.itemFromRow(groupId, rows)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
		if item == nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Storage) RefreshItemTypeHier(ctx context.Context) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("refreshing materialized view item_type_hier")
	}
	if _, err := s.db.Exec(ctx, SQLRefreshItemTypeHier); err != nil {
		return errors.Wrapf(err, "cannot refresh materialized view item_type_hier - %v", SQLRefreshItemTypeHier)
	}
	return nil
}

func (s *Storage) UpdateItemsGitlabTimestamp(ctx context.Context, groupId int64, now time.Time, gitlab *time.Time) error {
	var sqlstr string
	params := pgx.NamedArgs{
		"now":     now.Format("2006-01-02 15:04:05"),
		"library": groupId,
	}
	if gitlab != nil {
		sqlstr = SQLUpdateItemsGitlabTimestampWithFilter
		params["gitlab"] = gitlab.Format("2006-01-02 15:04:05")
	} else {
		sqlstr = SQLUpdateItemsGitlabTimestamp
	}
	if _, err := s.db.Exec(ctx, sqlstr, params); err != nil {
		return errors.Wrapf(err, "cannot execute %v - %v", sqlstr, params)
	}
	return nil
}
