package utility

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNullString_U(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want sql.NullString
	}{
		{name: "empty string is NULL", in: "", want: sql.NullString{}},
		{name: "non-empty string is valid", in: "abc", want: sql.NullString{String: "abc", Valid: true}},
		{name: "whitespace is kept as a valid value", in: " ", want: sql.NullString{String: " ", Valid: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NullString(tc.in))
		})
	}
}

func TestStringFromPtr_U(t *testing.T) {
	value, empty := "ops@panca.co.id", ""

	testCases := []struct {
		name string
		in   *string
		want string
	}{
		{name: "nil becomes empty string, not a panic", in: nil, want: ""},
		{name: "pointer to empty string stays empty", in: &empty, want: ""},
		{name: "pointer to a value is dereferenced", in: &value, want: "ops@panca.co.id"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, StringFromPtr(tc.in))
		})
	}
}

func TestPtrFromNullString_U(t *testing.T) {
	testCases := []struct {
		name string
		in   sql.NullString
		want *string
	}{
		{name: "invalid is nil", in: sql.NullString{String: "ignored", Valid: false}, want: nil},
		{name: "valid is a pointer to the value", in: sql.NullString{String: "abc", Valid: true}, want: StringPointer("abc")},
		{name: "valid empty string is a pointer to empty", in: sql.NullString{String: "", Valid: true}, want: StringPointer("")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PtrFromNullString(tc.in))
		})
	}
}

func TestNullInt64_U(t *testing.T) {
	testCases := []struct {
		name string
		in   int64
	}{
		{name: "zero is still valid", in: 0},
		{name: "positive", in: 42},
		{name: "negative", in: -7},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, sql.NullInt64{Int64: tc.in, Valid: true}, NullInt64(tc.in))
		})
	}
}

func TestPtrFromNullTime_U(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		in   sql.NullTime
		want *time.Time
	}{
		{name: "invalid is nil", in: sql.NullTime{Time: now, Valid: false}, want: nil},
		{name: "valid is a pointer to the value", in: sql.NullTime{Time: now, Valid: true}, want: &now},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PtrFromNullTime(tc.in))
		})
	}
}

func TestPtrFromNullInt64_U(t *testing.T) {
	v := int64(42)

	testCases := []struct {
		name string
		in   sql.NullInt64
		want *int64
	}{
		{name: "invalid is nil", in: sql.NullInt64{Int64: 42, Valid: false}, want: nil},
		{name: "valid is a pointer to the value", in: sql.NullInt64{Int64: 42, Valid: true}, want: &v},
		{name: "valid zero is still a pointer", in: sql.NullInt64{Int64: 0, Valid: true}, want: new(int64)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PtrFromNullInt64(tc.in))
		})
	}
}
