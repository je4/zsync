package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"emperror.dev/errors"
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
	sqlstr := fmt.Sprintf("INSERT INTO %s.tags (tag, meta, library) VALUES( $1, $2, $3)", s.dbSchema)
	params := []any{
		tag.Tag,
		metastr,
		groupId,
	}
	_, err = s.db.Exec(context.Background(), sqlstr, params...)
	if err != nil {
		if IsUniqueViolation(err, "pk_tags") {
			return nil
		}
		return errors.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (s *Storage) DeleteTag(groupId int64, tag string) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("deleting Tag %s", tag)
	}
	sqlstr := fmt.Sprintf("DELETE FROM %s.tags WHERE tag=$1 and library=$2", s.dbSchema)
	params := []any{
		tag,
		groupId,
	}
	if _, err := s.db.Exec(context.Background(), sqlstr, params...); err != nil {
		return errors.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}
