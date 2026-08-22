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
		if errors.Is(err, pgx.ErrNoRows) || err == sql.ErrNoRows {
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

func (s *Storage) LoadGroup(groupId int64) (*model.Group, error) {
	group := &model.Group{
		Id: groupId,
	}
	if s.Logger != nil {
		s.Logger.Debug().Msgf("loading Group #%v from database", groupId)
	}
	params := pgx.NamedArgs{"id": groupId}
	row := s.db.QueryRow(context.Background(), SQLLoadGroup, params)
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
		if !errors.Is(err, pgx.ErrNoRows) && err != sql.ErrNoRows {
			return nil, errors.Wrapf(err, "error scanning result of %s: %v", SQLLoadGroup, groupId)
		}
		active, direction, err := s.CreateEmptyGroup(groupId)
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

func (s *Storage) LoadGroups() ([]*model.Group, error) {
	if s.Logger != nil {
		s.Logger.Debug().Msgf("loading Groups from database")
	}
	rows, err := s.db.Query(context.Background(), SQLLoadGroups)
	if err != nil {
		return nil, errors.Wrapf(err, "error executing sql query: %v", SQLLoadGroups)
	}
	defer rows.Close()
	grps := []*model.Group{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, errors.Wrap(err, "cannot scan row")
		}
		grp, err := s.LoadGroup(id)
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

func (s *Storage) CreateEmptyGroup(groupId int64) (bool, model.SyncDirection, error) {
	active := s.newGroupActive
	direction := model.SyncDirection_ToLocal
	paramsGroup := pgx.NamedArgs{"id": groupId}
	_, err := s.db.Exec(context.Background(), SQLInsertEmptyGroup, paramsGroup)
	if err != nil {
		return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", SQLInsertEmptyGroup, groupId)
	}
	paramsSyncGroup := pgx.NamedArgs{
		"id":        groupId,
		"active":    active,
		"direction": model.SyncDirectionString[direction],
	}
	_, err = s.db.Exec(context.Background(), SQLInsertEmptySyncGroup, paramsSyncGroup)
	if err != nil {
		if IsUniqueViolation(err, "syncgroups_pkey") {
			var dirstr string
			if err := s.db.QueryRow(context.Background(), SQLGetSyncGroupActiveDirection, paramsGroup).Scan(&active, &dirstr); err != nil {
				return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", SQLGetSyncGroupActiveDirection, groupId)
			}
			direction = model.SyncDirectionId[dirstr]
		} else {
			return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", SQLInsertEmptySyncGroup, paramsSyncGroup)
		}
	}
	return active, direction, nil
}

func (s *Storage) ClearGroup(groupId int64) error {
	params := pgx.NamedArgs{"id": groupId}
	_, err := s.db.Exec(context.Background(), SQLClearGroup, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLClearGroup, groupId)
	}
	return nil
}

func (s *Storage) UpdateGroup(group *model.Group) error {
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
	_, err = s.db.Exec(context.Background(), SQLUpdateGroup, params)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", SQLUpdateGroup, params)
	}

	return nil
}

func (s *Storage) UpdateGroupGitlabTimestamp(groupId int64, t time.Time) error {
	params := pgx.NamedArgs{
		"gitlab": t.Format("2006-01-02 15:04:05"),
		"id":     groupId,
	}
	if _, err := s.db.Exec(context.Background(), SQLUpdateGroupGitlabTimestamp, params); err != nil {
		return errors.Wrapf(err, "cannot update timestamp for group #%v", groupId)
	}
	return nil
}
