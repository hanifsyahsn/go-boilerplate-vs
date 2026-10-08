package utility

import (
	"database/sql"
	"time"
)

// NullString wraps a string, treating "" as SQL NULL.
func NullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// StringFromPtr dereferences a *string, treating nil as "". For columns that
// are NOT NULL: callers that have already validated the value as required can
// pass it straight through, and a nil that slips past validation becomes an
// empty string rather than a nil-pointer panic.
func StringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// PtrFromNullString returns nil for an invalid sql.NullString, else a pointer
// to its value.
func PtrFromNullString(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

// NullInt64 wraps an int64 as an always-valid sql.NullInt64.
// PtrFromNullInt64 returns nil for an invalid NullInt64, else a pointer to its value.
func PtrFromNullInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func NullInt64(i int64) sql.NullInt64 {
	return sql.NullInt64{Int64: i, Valid: true}
}

// PtrFromNullTime returns nil for an invalid sql.NullTime, else a pointer to
// its value.
func PtrFromNullTime(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}
