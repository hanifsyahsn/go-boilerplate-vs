package testhelper

import (
	"context"
	"strings"

	"github.com/golang/mock/gomock"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
)

// FoldingQuerier is the auth-shared mock with FoldEmail answering like
// Postgres for ASCII (lowercase), for unit tests of a service that keys a
// limit on the folded email. A test about the fold itself sets its own
// FoldEmail expectation instead.
func FoldingQuerier(ctrl *gomock.Controller) *authsharedsqlc.MockQuerier {
	sq := authsharedsqlc.NewMockQuerier(ctrl)
	sq.EXPECT().FoldEmail(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, email string) (string, error) { return strings.ToLower(email), nil }).AnyTimes()
	return sq
}
