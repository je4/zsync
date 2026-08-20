package storage

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/lib/pq"
)

func TestIsEmptyResult(t *testing.T) {
	if !IsEmptyResult(sql.ErrNoRows) {
		t.Errorf("expected IsEmptyResult(sql.ErrNoRows) to be true")
	}
	if IsEmptyResult(errors.New("other error")) {
		t.Errorf("expected IsEmptyResult(other error) to be false")
	}
	if IsEmptyResult(nil) {
		t.Errorf("expected IsEmptyResult(nil) to be false")
	}
}

func TestIsUniqueViolation(t *testing.T) {
	pqErr := &pq.Error{
		Code:       "23505",
		Constraint: "pk_tags",
	}
	if !IsUniqueViolation(pqErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with matching constraint to be true")
	}
	if !IsUniqueViolation(pqErr, "") {
		t.Errorf("expected IsUniqueViolation with empty constraint to be true")
	}
	if IsUniqueViolation(pqErr, "other_constraint") {
		t.Errorf("expected IsUniqueViolation with different constraint to be false")
	}

	otherPqErr := &pq.Error{
		Code:       "23503",
		Constraint: "pk_tags",
	}
	if IsUniqueViolation(otherPqErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with code 23503 to be false")
	}

	regularErr := errors.New("some error")
	if IsUniqueViolation(regularErr, "") {
		t.Errorf("expected IsUniqueViolation for non-pq error to be false")
	}
}
