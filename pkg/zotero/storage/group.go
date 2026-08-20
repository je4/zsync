package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func (s *Storage) groupFromRow(rowss any) (*model.Group, error) {
	group := model.Group{}
	var datastr sql.NullString
	var gitlab sql.NullTime
	switch r := rowss.(type) {
	case *sql.Row:
		if err := r.Scan(&group.Id, &group.Version, &group.Meta.Created, &group.Meta.LastModified, &datastr, &group.Deleted, &group.ItemVersion, &group.CollectionVersion, &group.TagVersion, &gitlab); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, errors.Wrapf(err, "cannot scan row")
		}
	case *sql.Rows:
		if err := r.Scan(&group.Id, &group.Version, &group.Meta.Created, &group.Meta.LastModified, &datastr, &group.Deleted, &group.ItemVersion, &group.CollectionVersion, &group.TagVersion, &gitlab); err != nil {
			return nil, errors.Wrapf(err, "cannot scan row")
		}
	default:
		return nil, errors.Errorf("unknown row type: %T", rowss)
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
	sqlstr := fmt.Sprintf("SELECT version, created, modified, data, active, direction, tags,"+
		" itemversion, collectionversion, tagversion, gitlab"+
		" FROM %s.groups g, %s.syncgroups sg WHERE g.id=sg.id AND g.id=$1", s.dbSchema, s.dbSchema)
	row := s.db.QueryRow(sqlstr, groupId)
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
		if err != sql.ErrNoRows {
			return nil, errors.Wrapf(err, "error scanning result of %s: %v", sqlstr, groupId)
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
	sqlstr := fmt.Sprintf("SELECT id FROM %s.syncgroups sg WHERE sg.active=true", s.dbSchema)
	rows, err := s.db.Query(sqlstr)
	if err != nil {
		return nil, errors.Wrapf(err, "error executing sql query: %v", sqlstr)
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
	sqlstr := fmt.Sprintf("INSERT INTO %s.groups (id,version,created,modified) VALUES($1, 0, NOW(), NOW())", s.dbSchema)
	_, err := s.db.Exec(sqlstr, groupId)
	if err != nil {
		return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
	}
	sqlstr = fmt.Sprintf("INSERT INTO %s.syncgroups(id,active,direction) VALUES($1, $2, $3)", s.dbSchema)
	params := []any{
		groupId,
		active,
		model.SyncDirectionString[direction],
	}
	_, err = s.db.Exec(sqlstr, params...)
	if err != nil {
		if IsUniqueViolation(err, "syncgroups_pkey") {
			var dirstr string
			sqlstr := fmt.Sprintf("SELECT active, direction FROM %s.syncgroups WHERE id=$1", s.dbSchema)
			if err := s.db.QueryRow(sqlstr, groupId).Scan(&active, &dirstr); err != nil {
				return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
			}
			direction = model.SyncDirectionId[dirstr]
		} else {
			return false, model.SyncDirection_None, errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
		}
	}
	return active, direction, nil
}

func (s *Storage) ClearGroup(groupId int64) error {
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET version=0, modified=created,"+
		" itemversion=0, collectionversion=0"+
		" WHERE id=$1", s.dbSchema)
	_, err := s.db.Exec(sqlstr, groupId)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
	}
	return nil
}

func (s *Storage) UpdateGroup(group *model.Group) error {
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET version=$1, created=$2, modified=$3, data=$4, deleted=$5,"+
		" itemversion=$6, collectionversion=$7, tagversion=$8"+
		" WHERE id=$9", s.dbSchema)
	data, err := json.MarshalIndent(group.Data, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "cannot marshal Group data")
	}

	params := []any{
		group.Version,
		group.Meta.Created,
		group.Meta.LastModified,
		data,
		group.Deleted,
		group.ItemVersion,
		group.CollectionVersion,
		group.TagVersion,
		group.Id,
	}
	_, err = s.db.Exec(sqlstr, params...)
	if err != nil {
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return nil
}

func (s *Storage) UpdateGroupGitlabTimestamp(groupId int64, t time.Time) error {
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET gitlab=TO_TIMESTAMP($1, 'YYYY-MM-DD HH24:MI:SS') WHERE id=$2", s.dbSchema)
	if _, err := s.db.Exec(sqlstr, t.Format("2006-01-02 15:04:05"), groupId); err != nil {
		return errors.Wrapf(err, "cannot update timestamp for group #%v", groupId)
	}
	return nil
}
