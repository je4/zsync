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

func (s *Storage) collectionFromRow(groupId int64, rowss any) (*model.Collection, error) {
	coll := model.Collection{}
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var gitlab sql.NullTime
	switch r := rowss.(type) {
	case *sql.Row:
		if err := r.Scan(&coll.Key, &coll.Version, &datastr, &metastr, &coll.Deleted, &sync, &gitlab); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, errors.Wrapf(err, "cannot scan row")
		}
	case *sql.Rows:
		if err := r.Scan(&coll.Key, &coll.Version, &datastr, &metastr, &coll.Deleted, &sync, &gitlab); err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
	default:
		return nil, errors.Errorf("unknown row type: %v", reflect.TypeOf(rowss).String())
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
	sqlstr := fmt.Sprintf("INSERT INTO %s.collections (key, version, library, sync, data, deleted) VALUES( $1, $2, $3, $4, $5, false)", s.dbSchema)
	params := []any{
		coll.Key,
		0,
		coll.Library.Id,
		"new",
		string(jsonstr),
	}
	_, err = s.db.Exec(sqlstr, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
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
	sqlstr := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s.collection_name_hier WITH DATA", s.dbSchema)
	if _, err := s.db.Exec(sqlstr); err != nil {
		return errors.Wrapf(err, "cannot refresh materialized view collection_name_hier - %v", sqlstr)
	}
	return nil
}

func (s *Storage) CreateEmptyCollection(groupId int64, collectionId string) error {
	sqlstr := fmt.Sprintf("INSERT INTO %s.collections (key, version, library, sync) VALUES( $1, 0, $2, $3)", s.dbSchema)
	params := []any{
		collectionId,
		groupId,
		model.SyncStatusString[model.SyncStatus_New],
	}
	_, err := s.db.Exec(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) GetCollectionVersion(groupId int64, collectionId string) (int64, model.SyncStatus, error) {
	sqlstr := fmt.Sprintf("SELECT version, sync FROM %s.collections WHERE library=$1 AND key=$2", s.dbSchema)
	params := []any{
		groupId,
		collectionId,
	}
	var version int64
	var sync string
	var status model.SyncStatus
	err := s.db.QueryRow(sqlstr, params...).Scan(&version, &sync)
	switch {
	case err == sql.ErrNoRows:
		if err := s.CreateEmptyCollection(groupId, collectionId); err != nil {
			return 0, model.SyncStatus_Incomplete, errors.Wrapf(err, "cannot create new collection")
		}
		version = 0
		status = model.SyncStatus_New
	case err != nil:
		return 0, model.SyncStatus_Incomplete, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	case err == nil:
		status = model.SyncStatusId[sync]
	}
	return version, status, nil
}

func (s *Storage) GetCollections(groupId int64, objectKeys []string) (*[]model.Collection, error) {
	if len(objectKeys) == 0 {
		return &[]model.Collection{}, nil
	}
	params := []any{
		groupId,
	}
	pstr := []string{}
	for _, val := range objectKeys {
		params = append(params, val)
		pstr = append(pstr, fmt.Sprintf("$%v", len(params)))
	}
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, deleted, sync, gitlab FROM %s.collections WHERE library=$1 AND key IN (%s)", s.dbSchema, strings.Join(pstr, ","))
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return &[]model.Collection{}, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
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
	sqlstr := fmt.Sprintf("SELECT key, version FROM %s.collections WHERE library=$1 AND version > $2", s.dbSchema)
	params := []any{
		groupId,
		sinceVersion,
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

func (s *Storage) GetCollectionByKey(groupId int64, key string) (*model.Collection, error) {
	sqlstr := fmt.Sprintf("SELECT cs.key,cs.version,cs.data,cs.meta,cs.deleted,cs.sync,cs.gitlab"+
		" FROM %s.collections cs WHERE cs.library=$1 AND cs.key=$2", s.dbSchema)

	params := []any{
		groupId,
		key,
	}
	var coll model.Collection
	var datastr sql.NullString
	var metastr sql.NullString
	var sync string
	var gitlab sql.NullTime
	if err := s.db.QueryRow(sqlstr, params...).
		Scan(&(coll.Key), &(coll.Version), &datastr, &metastr, &(coll.Deleted), &sync, &gitlab); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "cannot get collection: %v - %v", sqlstr, params)
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

	sqlstr := fmt.Sprintf("SELECT cs.key,cs.version,cs.data,cs.meta,cs.deleted,cs.sync,cs.gitlab"+
		" FROM %s.collections cs, %s.collection_name_hier cnh"+
		" WHERE cs.key=cnh.key AND cs.library=$1 AND cnh.name=$2", s.dbSchema, s.dbSchema)

	params := []any{
		groupId,
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
	var gitlab sql.NullTime
	if err := s.db.QueryRow(sqlstr, params...).
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
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET version=$1, sync=$2, data=$3, meta=$4, deleted=$5, modified=NOW() WHERE library=$6 AND key=$7", s.dbSchema)
	params := []any{
		collection.Version,
		model.SyncStatusString[collection.Status],
		data,
		meta,
		collection.Deleted,
		groupId,
		collection.Key,
	}
	_, err = s.db.Exec(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) DeleteCollection(groupId int64, key string) error {
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET deleted=true, sync=$1, modified=NOW() WHERE library=$2 AND key=$3", s.dbSchema)
	params := []any{
		model.SyncStatusString[model.SyncStatus_Modified],
		groupId,
		key,
	}
	if _, err := s.db.Exec(sqlstr, params...); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) IterateCollections(groupId int64, after *time.Time, f func(coll *model.Collection) error) error {
	sqlstr0 := fmt.Sprintf("SELECT COUNT(*) "+
		"FROM %s.collections WHERE library=$1 AND deleted=false", s.dbSchema)
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, deleted, sync, gitlab "+
		"FROM %s.collections WHERE library=$1 AND deleted=false", s.dbSchema)
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
		s.Logger.Info().Msgf("%v collections found", num)
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get item")
		}
		if err := f(coll); err != nil {
			return errors.Wrapf(err, "error in callback for %v", coll.Key)
		}
	}
	return nil
}

func (s *Storage) IterateCollectionsAll(groupId int64, after *time.Time, f func(coll *model.Collection) error) error {
	sqlstr0 := fmt.Sprintf("SELECT COUNT(*) "+
		"FROM %s.collections WHERE library=$1", s.dbSchema)
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, deleted, sync, gitlab "+
		"FROM %s.collections WHERE library=$1", s.dbSchema)
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
		s.Logger.Info().Msgf("%v collections found", num)
	}
	rows, err := s.db.Query(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s", sqlstr)
	}
	defer rows.Close()

	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return errors.Wrapf(err, "cannot get collection")
		}
		if err := f(coll); err != nil {
			return errors.Wrapf(err, "error in callback for %v", coll.Key)
		}
	}
	return nil
}

func (s *Storage) GetModifiedCollections(groupId int64) ([]*model.Collection, error) {
	sqlstr := fmt.Sprintf("SELECT key, version, data, meta, deleted, sync, gitlab"+
		" FROM %s.collections"+
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

	colls := []*model.Collection{}
	for rows.Next() {
		coll, err := s.collectionFromRow(groupId, rows)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
		colls = append(colls, coll)
	}
	return colls, nil
}

func (s *Storage) UpdateCollectionsGitlabTimestamp(groupId int64, now time.Time, gitlab *time.Time) error {
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET gitlab=TO_TIMESTAMP($1, 'YYYY-MM-DD HH24:MI:SS') "+
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
