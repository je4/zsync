package storage

import (
	"database/sql"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/lib/pq"
)

type Storage struct {
	db             *sql.DB
	dbSchema       string
	newGroupActive bool
	Logger         zLogger.ZLogger
}

func NewStorage(db *sql.DB, dbSchema string, newGroupActive bool, logger zLogger.ZLogger) *Storage {
	return &Storage{
		db:             db,
		dbSchema:       dbSchema,
		newGroupActive: newGroupActive,
		Logger:         logger,
	}
}

func (s *Storage) GetDB() *sql.DB {
	return s.db
}

func (s *Storage) GetSchema() string {
	return s.dbSchema
}

func IsEmptyResult(err error) bool {
	return err == sql.ErrNoRows
}

func IsUniqueViolation(err error, constraint string) bool {
	pqErr, ok := err.(*pq.Error)
	if !ok {
		return false
	}
	if pqErr.Code != "23505" {
		return false
	}
	if constraint != "" && pqErr.Constraint != constraint {
		return false
	}
	return true
}
