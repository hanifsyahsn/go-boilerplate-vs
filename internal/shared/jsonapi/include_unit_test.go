package jsonapi

import (
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestResolveIncludePolicy(t *testing.T) {
	cfg := IncludeConfig{
		Allowed: []string{"created_by", "department", "created_by.company"},
		Default: []string{"created_by"},
	}

	testCases := []struct {
		name    string
		raw     string
		present bool
		assert  func(t *testing.T, p IncludePolicy, err *apperror.AppError)
	}{
		{
			name:    "no ?include -> default is applied",
			present: false,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, err)
				require.Equal(t, IncludePolicy{"memos": {"created_by": {}}}, p)
			},
		},
		{
			name:    "?include= (empty) -> nothing included, nil policy",
			raw:     "",
			present: true,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, err)
				require.Nil(t, p)
			},
		},
		{
			name:    "?include=created_by -> exactly that",
			raw:     "created_by",
			present: true,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, err)
				require.Equal(t, IncludePolicy{"memos": {"created_by": {}}}, p)
			},
		},
		{
			name:    "?include=created_by,department -> both, order-independent",
			raw:     "department, created_by",
			present: true,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, err)
				require.Equal(t, IncludePolicy{"memos": {"created_by": {}, "department": {}}}, p)
			},
		},
		{
			name:    "blank segments from stray commas are skipped",
			raw:     "created_by,, ,",
			present: true,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, err)
				require.Equal(t, IncludePolicy{"memos": {"created_by": {}}}, p)
			},
		},
		{
			name:    "a dotted path builds a nested tree",
			raw:     "created_by.company",
			present: true,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, err)
				require.Equal(t, IncludePolicy{"memos": {"created_by": {"company": {}}}}, p)
			},
		},
		{
			name:    "unknown include -> 400 on the include parameter",
			raw:     "created_by,bogus",
			present: true,
			assert: func(t *testing.T, p IncludePolicy, err *apperror.AppError) {
				require.Nil(t, p)
				require.NotNil(t, err)
				require.Equal(t, apperror.CodeBadRequest, err.Code)
				require.Contains(t, err.Message, `"bogus"`)
				require.Equal(t, &apperror.Source{Parameter: "include"}, err.Source)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ResolveIncludePolicy(tc.raw, tc.present, "memos", cfg)
			tc.assert(t, p, err)
		})
	}
}
