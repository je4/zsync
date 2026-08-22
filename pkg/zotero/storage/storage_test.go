package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsEmptyResult(t *testing.T) {
	if !IsEmptyResult(sql.ErrNoRows) {
		t.Errorf("expected IsEmptyResult(sql.ErrNoRows) to be true")
	}
	if !IsEmptyResult(pgx.ErrNoRows) {
		t.Errorf("expected IsEmptyResult(pgx.ErrNoRows) to be true")
	}
	if !IsEmptyResult(fmt.Errorf("wrapped error: %w", pgx.ErrNoRows)) {
		t.Errorf("expected IsEmptyResult(wrapped pgx.ErrNoRows) to be true")
	}
	if IsEmptyResult(errors.New("other error")) {
		t.Errorf("expected IsEmptyResult(other error) to be false")
	}
	if IsEmptyResult(nil) {
		t.Errorf("expected IsEmptyResult(nil) to be false")
	}
}

func TestIsUniqueViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "pk_tags",
	}
	if !IsUniqueViolation(pgErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with matching constraint to be true")
	}
	if !IsUniqueViolation(pgErr, "") {
		t.Errorf("expected IsUniqueViolation with empty constraint to be true")
	}
	if IsUniqueViolation(pgErr, "other_constraint") {
		t.Errorf("expected IsUniqueViolation with different constraint to be false")
	}

	wrappedPgErr := fmt.Errorf("wrapped pg error: %w", pgErr)
	if !IsUniqueViolation(wrappedPgErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with wrapped error to be true")
	}

	otherPgErr := &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "pk_tags",
	}
	if IsUniqueViolation(otherPgErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with code 23503 to be false")
	}

	regularErr := errors.New("some error")
	if IsUniqueViolation(regularErr, "") {
		t.Errorf("expected IsUniqueViolation for non-pg error to be false")
	}
	if IsUniqueViolation(nil, "") {
		t.Errorf("expected IsUniqueViolation for nil error to be false")
	}
}

func TestStorageAccessors(t *testing.T) {
	st := NewStorage(nil, true, nil)
	if st.GetDB() != nil {
		t.Errorf("expected GetDB() to be nil")
	}
}
