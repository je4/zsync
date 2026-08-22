package storage

import (
	"context"
	"encoding/json/v2"

	"emperror.dev/errors"
	"github.com/jackc/pgx/v5"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func (s *Storage) CreateTag(groupId int64, tag model.Tag) error {
	if s.Logger != nil {
		s.Logger.Debug().Msgf("Creating Tag %s", tag.Tag)
	}
	metastr, err := json.Marshal(tag.Meta)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal meta %v", tag.Meta)
	}
	params := pgx.NamedArgs{
		"tag":     tag.Tag,
		"meta":    metastr,
		"library": groupId,
	}
	_, err = s.db.Exec(context.Background(), SQLInsertTag, params)
	if err != nil {
		if IsUniqueViolation(err, "pk_tags") {
			return nil
		}
		return errors.Wrapf(err, "cannot execute %s: %v", SQLInsertTag, params)
	}
	return nil
}

func (s *Storage) DeleteTag(groupId int64, tag string) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("deleting Tag %s", tag)
	}
	params := pgx.NamedArgs{
		"tag":     tag,
		"library": groupId,
	}
	if _, err := s.db.Exec(context.Background(), SQLDeleteTag, params); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", SQLDeleteTag, params)
	}
	return nil
}
