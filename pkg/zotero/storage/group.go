package storage

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"time"

	"emperror.dev/errors"
	"github.com/jackc/pgx/v5"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func (s *Storage) groupFromRow(row pgx.Row) (*model.Group, error) {
	group := model.Group{}
	var datastr sql.NullString
	var gitlab sql.NullTime
	if err := row.Scan(&group.Id, &group.Version, &group.Meta.Created, &group.Meta.LastModified, &datastr, &group.Deleted, &group.ItemVersion, &group.CollectionVersion, &group.TagVersion, &gitlab); err != nil {
		if IsEmptyResult(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "cannot scan row")
	}
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &group.Data); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal group data: %s", datastr.String)
		}
	}
	if gitlab.Valid {
		group.Gitlab = &gitlab.Time
	}
	group.Init()
	return &group, nil
}

func (s *Storage) GetGroup(ctx context.Context, groupId int64) (*model.Group, error) {
	group := &model.Group{
		Id: groupId,
	}
	if s.Logger != nil {
		s.Logger.Debug().Msgf("loading Group #%v from database", groupId)
	}
	params := pgx.NamedArgs{"id": groupId}
	row := s.db.QueryRow(ctx, SQLGetGroup, params)
	var jsonstr sql.NullString
	var directionstr string
	var gitlab sql.NullTime
	err := row.Scan(&group.Version,
		&group.Meta.Created,
		&group.Meta.LastModified,
		&jsonstr,
		&group.Active,
		&directionstr,
		&group.SyncTags,
		&group.ItemVersion,
		&group.CollectionVersion,
		&group.TagVersion,
		&gitlab)
	if err != nil {
		if !IsEmptyResult(err) {
			return nil, errors.Wrapf(err, "error scanning result of %s: %v", SQLGetGroup, groupId)
		}
		active, direction, err := s.CreateEmptyGroup(ctx, groupId)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create empty Group %v", groupId)
		}
		group.Active = active
		group.Direction = direction
	} else {
		if gitlab.Valid {
			group.Gitlab = &gitlab.Time
		}
		group.Direction = model.SyncDirectionId[directionstr]
	}
	if jsonstr.Valid {
		err = json.Unmarshal([]byte(jsonstr.String), &group.Data)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal Group data %s", jsonstr.String)
		}
	}
	group.Init()
	return group, nil
}

func (s *Storage) GetGroups(ctx context.Context) ([]*model.Group, error) {
	if s.Logger != nil {
		s.Logger.Debug().Msgf("loading Groups from database")
	}
	rows, err := s.db.Query(ctx, SQLGetGroups)
	if err != nil {
		return nil, errors.Wrapf(err, "error executing sql query: %v", SQLGetGroups)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, errors.Wrap(err, "cannot scan row")
		}
		ids = append(ids, id)
	}
	rows.Close()

	grps := []*model.Group{}
	for _, id := range ids {
		grp, err := s.GetGroup(ctx, id)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Error().Msgf("error loading Group #%v: %v", id, err)
			}
			continue
		}
		if s.Logger != nil {
			s.Logger.Info().Msgf("Group #%v - %v loaded", grp.Id, grp.Data.Name)
		}
		grps = append(grps, grp)
	}

	return grps, nil
}

func (s *Storage) CreateEmptyGroup(ctx context.Context, groupId int64) (bool, model.SyncDirection, error) {
	active := s.newGroupActive
	direction := model.SyncDirection_ToLocal
	paramsGroup := pgx.NamedArgs{"id": groupId}
	_, err := s.db.Exec(ctx, SQLInsertEmptyGroup, paramsGroup)
	if err != nil {
		return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", SQLInsertEmptyGroup, groupId)
	}
	paramsSyncGroup := pgx.NamedArgs{
		"id":        groupId,
		"active":    active,
		"direction": model.SyncDirectionString[direction],
	}
	_, err = s.db.Exec(ctx, SQLInsertEmptySyncGroup, paramsSyncGroup)
	if err != nil {
		if IsUniqueViolation(err, "syncgroups_pkey") {
			var dirstr string
			if err := s.db.QueryRow(ctx, SQLGetSyncGroupActiveDirection, paramsGroup).Scan(&active, &dirstr); err != nil {
				return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", SQLGetSyncGroupActiveDirection, groupId)
			}
			direction = model.SyncDirectionId[dirstr]
		} else {
			return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", SQLInsertEmptySyncGroup, paramsSyncGroup)
		}
	}
	return active, direction, nil
}

func (s *Storage) ClearGroup(ctx context.Context, groupId int64) error {
	params := pgx.NamedArgs{"id": groupId}
	_, err := s.db.Exec(ctx, SQLClearGroup, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLClearGroup, groupId)
	}
	return nil
}

func (s *Storage) UpdateGroup(ctx context.Context, group *model.Group) error {
	data, err := json.Marshal(group.Data, jsontext.WithIndent("  "))
	if err != nil {
		return errors.Wrapf(err, "cannot marshal Group data")
	}

	params := pgx.NamedArgs{
		"version":           group.Version,
		"created":           group.Meta.Created,
		"modified":          group.Meta.LastModified,
		"data":              data,
		"deleted":           group.Deleted,
		"itemversion":       group.ItemVersion,
		"collectionversion": group.CollectionVersion,
		"tagversion":        group.TagVersion,
		"id":                group.Id,
	}
	_, err = s.db.Exec(ctx, SQLUpdateGroup, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLUpdateGroup, params)
	}

	return nil
}

func (s *Storage) UpdateGroupGitlabTimestamp(ctx context.Context, groupId int64, t time.Time) error {
	params := pgx.NamedArgs{
		"gitlab": t.Format("2006-01-02 15:04:05"),
		"id":     groupId,
	}
	if _, err := s.db.Exec(ctx, SQLUpdateGroupGitlabTimestamp, params); err != nil {
		return errors.Wrapf(err, "cannot update timestamp for group #%v", groupId)
	}
	return nil
}
