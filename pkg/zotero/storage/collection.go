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

func (s *Storage) collectionFromRow(groupId int64, row pgx.Row) (*model.Collection, error) {
	coll := model.Collection{}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var gitlab sql.NullTime
	if err := row.Scan(&coll.Key, &coll.Version, &datastr, &metastr, &coll.Deleted, &sync, &gitlab); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "cannot scan row")
	}
	if gitlab.Valid {
		coll.Gitlab = &gitlab.Time
	}
	coll.Status = model.SyncStatusId[sync]
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &coll.Data); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal data %s", datastr.String)
		}
	} else {
		if coll.Deleted || coll.Status == model.SyncStatus_Incomplete {
			return nil, nil
		}
		return nil, errors.Errorf("collection has no data %v.%v", groupId, coll.Key)
	}
	if metastr.Valid {
		if err := json.Unmarshal([]byte(metastr.String), &coll.Meta); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal meta %s", metastr.String)
		}
	}
	if coll.Data.Name == "" {
		coll.Data.Name = coll.Key
	}
	coll.Library.Id = groupId
	coll.Library.Type = "group"

	return &coll, nil
}

func (s *Storage) CreateCollection(groupId int64, collectionData *model.CollectionData) (*model.Collection, error) {
	if collectionData.Key == "" {
		collectionData.Key = model.CreateKey()
	}
	coll := &model.Collection{
		Key:     collectionData.Key,
		Version: 0,
		Library: model.Library{
			Id:   groupId,
			Type: "group",
		},
		Meta: model.CollectionMeta{},
		Data: *collectionData,
	}
	jsonstr, err := json.Marshal(collectionData)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot marshal collection data %v", collectionData)
	}
	params := pgx.NamedArgs{
		"key":     coll.Key,
		"version": 0,
		"library": coll.Library.Id,
		"sync":    "new",
		"data":    string(jsonstr),
	}
	_, err = s.db.Exec(context.Background(), SQLInsertCollection, params)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLInsertCollection, params)
	}
	if err := s.RefreshCollectionNameHier(); err != nil {
		return nil, err
	}

	return coll, nil
}

func (s *Storage) RefreshCollectionNameHier() error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("refreshing materialized view collection_name_hier")
	}
	if _, err := s.db.Exec(context.Background(), SQLRefreshCollectionNameHier); err != nil {
		return errors.Wrapf(err, "cannot refresh materialized view collection_name_hier - %v", SQLRefreshCollectionNameHier)
	}
	return nil
}

func (s *Storage) CreateEmptyCollection(groupId int64, collectionId string) error {
	params := pgx.NamedArgs{
		"key":     collectionId,
		"library": groupId,
		"sync":    model.SyncStatusString[model.SyncStatus_Incomplete],
	}
	_, err := s.db.Exec(context.Background(), SQLInsertEmptyCollection, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLInsertEmptyCollection, params)
	}
	return nil
}

func (s *Storage) GetCollectionVersion(groupId int64, collectionId string) (int64, model.SyncStatus, error) {
	params := pgx.NamedArgs{
		"library": groupId,
		"key":     collectionId,
	}
	var version int64
	var sync string
	var status model.SyncStatus
	err := s.db.QueryRow(context.Background(), SQLGetCollectionVersion, params).Scan(&version, &sync)
	switch {
	case errors.Is(err, pgx.ErrNoRows) || err == sql.ErrNoRows:
		if err := s.CreateEmptyCollection(groupId, collectionId); err != nil {
			return 0, model.SyncStatus_Incomplete, errors.Wrapf(err, "cannot create new collection")
		}
		version = 0
		status = model.SyncStatus_Incomplete
	case err != nil:
		return 0, model.SyncStatus_Incomplete, errors.Wrapf(err, "cannot execute %s: %v", SQLGetCollectionVersion, params)
	case err == nil:
		status = model.SyncStatusId[sync]
	}
	return version, status, nil
}

func (s *Storage) GetCollections(groupId int64, objectKeys []string) (*[]model.Collection, error) {
	if len(objectKeys) == 0 {
		return &[]model.Collection{}, nil
	}
	params := pgx.NamedArgs{
		"library": groupId,
		"keys":    objectKeys,
	}
	rows, err := s.db.Query(context.Background(), SQLGetCollections, params)
	if err != nil {
		return &[]model.Collection{}, errors.Wrapf(err, "cannot execute %s: %v", SQLGetCollections, params)
	}
	defer rows.Close()

	result := []model.Collection{}
	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
		result = append(result, *coll)
	}
	return &result, nil
}

func (s *Storage) GetCollectionVersions(groupId int64, sinceVersion int64) (*map[string]int64, int64, error) {
	params := pgx.NamedArgs{
		"library":      groupId,
		"sinceVersion": sinceVersion,
	}
	rows, err := s.db.Query(context.Background(), SQLGetCollectionVersions, params)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "cannot execute %s: %v", SQLGetCollectionVersions, params)
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

func (s *Storage) GetCollectionByKey(groupId int64, key string) (*model.Collection, error) {
	params := pgx.NamedArgs{
		"library": groupId,
		"key":     key,
	}
	var coll model.Collection
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var gitlab sql.NullTime
	if err := s.db.QueryRow(context.Background(), SQLGetCollectionByKey, params).
		Scan(&(coll.Key), &(coll.Version), &datastr, &metastr, &(coll.Deleted), &sync, &gitlab); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "cannot get collection: %v - %v", SQLGetCollectionByKey, params)
	}
	if gitlab.Valid {
		coll.Gitlab = &gitlab.Time
	}
	coll.Status = model.SyncStatusId[sync]
	coll.Library.Id = groupId
	coll.Library.Type = "group"
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &(coll.Data)); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal collection data - %v", datastr)
		}
	}
	if metastr.Valid {
		if err := json.Unmarshal([]byte(metastr.String), &(coll.Meta)); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal collection metadata - %v", metastr)
		}
	}

	return &coll, nil
}

func (s *Storage) GetCollectionByName(groupId int64, name string, parentKey string) (*model.Collection, error) {
	coll := model.Collection{
		Key:     "",
		Version: 0,
		Library: model.Library{
			Id:   groupId,
			Type: "group",
		},
		Status: model.SyncStatus_New,
	}

	var sqlstr string
	params := pgx.NamedArgs{
		"library": groupId,
		"name":    name,
	}
	if parentKey != "" {
		sqlstr = SQLGetCollectionByName
		params["parent"] = parentKey
	} else {
		sqlstr = SQLGetCollectionByNameTop
	}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var gitlab sql.NullTime
	if err := s.db.QueryRow(context.Background(), sqlstr, params).
		Scan(&coll.Key, &coll.Version, &datastr, &metastr, &coll.Deleted, &sync, &gitlab); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "cannot get collection: %v - %v", sqlstr, params)
	}
	if gitlab.Valid {
		coll.Gitlab = &gitlab.Time
	}
	coll.Status = model.SyncStatusId[sync]
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &coll.Data); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal collection data - %v", datastr)
		}
	}
	if metastr.Valid {
		if err := json.Unmarshal([]byte(metastr.String), &coll.Meta); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal collection metadata - %v", metastr)
		}
	}

	return &coll, nil
}

func (s *Storage) UpdateCollection(groupId int64, collection *model.Collection) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Updating Collection [#%s]", collection.Key)
	}
	data, err := json.Marshal(collection.Data)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal data %v", collection.Data)
	}
	meta, err := json.Marshal(collection.Meta)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal meta %v", collection.Meta)
	}
	params := pgx.NamedArgs{
		"version": collection.Version,
		"sync":    model.SyncStatusString[collection.Status],
		"data":    data,
		"meta":    meta,
		"deleted": collection.Deleted,
		"library": groupId,
		"key":     collection.Key,
	}
	_, err = s.db.Exec(context.Background(), SQLUpdateCollection, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLUpdateCollection, params)
	}
	return nil
}

func (s *Storage) DeleteCollection(groupId int64, key string) error {
	params := pgx.NamedArgs{
		"sync":    model.SyncStatusString[model.SyncStatus_Modified],
		"library": groupId,
		"key":     key,
	}
	if _, err := s.db.Exec(context.Background(), SQLDeleteCollection, params); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", SQLDeleteCollection, params)
	}
	return nil
}

func (s *Storage) IterateCollections(groupId int64, after *time.Time, f func(coll *model.Collection) error) error {
	var sqlstr0, sqlstr string
	params := pgx.NamedArgs{
		"library": groupId,
	}
	if after != nil {
		sqlstr0 = SQLIterateCollectionsAfterCount
		sqlstr = SQLIterateCollectionsAfter
		params["after"] = after.Format("2006-01-02 15:04:05")
	} else {
		sqlstr0 = SQLIterateCollectionsCount
		sqlstr = SQLIterateCollections
	}
	var num int64
	if err := s.db.QueryRow(context.Background(), sqlstr0, params).Scan(&num); err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr0)
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("%v collections found", num)
	}
	rows, err := s.db.Query(context.Background(), sqlstr, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get collection")
		}
		if coll == nil {
			continue
		}
		if err := f(coll); err != nil {
			return errors.Wrapf(err, "error in callback for %v", coll.Key)
		}
	}
	return nil
}

func (s *Storage) IterateCollectionsAll(groupId int64, after *time.Time, f func(coll *model.Collection) error) error {
	var sqlstr0, sqlstr string
	params := pgx.NamedArgs{
		"library": groupId,
	}
	if after != nil {
		sqlstr0 = SQLIterateCollectionsAllAfterCount
		sqlstr = SQLIterateCollectionsAllAfter
		params["after"] = after.Format("2006-01-02 15:04:05")
	} else {
		sqlstr0 = SQLIterateCollectionsAllCount
		sqlstr = SQLIterateCollectionsAll
	}
	var num int64
	if err := s.db.QueryRow(context.Background(), sqlstr0, params).Scan(&num); err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr0)
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("%v collections found", num)
	}
	rows, err := s.db.Query(context.Background(), sqlstr, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get collection")
		}
		if coll == nil {
			continue
		}
		if err := f(coll); err != nil {
			return errors.Wrapf(err, "error in callback for %v", coll.Key)
		}
	}
	return nil
}

func (s *Storage) GetModifiedCollections(groupId int64) ([]*model.Collection, error) {
	params := pgx.NamedArgs{
		"library":      groupId,
		"syncNew":      "new",
		"syncModified": "modified",
	}
	rows, err := s.db.Query(context.Background(), SQLGetModifiedCollections, params)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", SQLGetModifiedCollections, params)
	}
	defer rows.Close()

	colls := []*model.Collection{}
	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
		if coll == nil {
			continue
		}
		colls = append(colls, coll)
	}
	return colls, nil
}

func (s *Storage) UpdateCollectionsGitlabTimestamp(groupId int64, now time.Time, gitlab *time.Time) error {
	var sqlstr string
	params := pgx.NamedArgs{
		"now":     now.Format("2006-01-02 15:04:05"),
		"library": groupId,
	}
	if gitlab != nil {
		sqlstr = SQLUpdateCollectionsGitlabTimestampWithFilter
		params["gitlab"] = gitlab.Format("2006-01-02 15:04:05")
	} else {
		sqlstr = SQLUpdateCollectionsGitlabTimestamp
	}
	if _, err := s.db.Exec(context.Background(), sqlstr, params); err != nil {
		return errors.Wrapf(err, "cannot execute %v - %v", sqlstr, params)
	}
	return nil
}
