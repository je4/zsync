package storage

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/utils/v2/pkg/zLogger"
)

type Storage struct {
	db             *pgxpool.Pool
	newGroupActive bool
	Logger         zLogger.ZLogger
}

func NewStorage(db *pgxpool.Pool, newGroupActive bool, logger zLogger.ZLogger) *Storage {
	return &Storage{
		db:             db,
		newGroupActive: newGroupActive,
		Logger:         logger,
	}
}

func (s *Storage) GetDB() *pgxpool.Pool {
	return s.db
}

func (s *Storage) GetPool() *pgxpool.Pool {
	return s.db
}

func IsEmptyResult(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

func IsUniqueViolation(err error, constraint string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if constraint != "" && pgErr.ConstraintName != constraint {
		return false
	}
	return true
}
