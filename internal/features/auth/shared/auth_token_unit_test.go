package shared

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/stretchr/testify/require"
)

func TestIssueAuthToken_U(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

	testCases := []struct {
		name    string
		stub    func(q *sqlc.MockQuerier, hash *string)
		wantErr string
	}{
		{
			name: "the user's unused tokens of that purpose are spent, then the new token's hash is stored with its expiry",
			stub: func(q *sqlc.MockQuerier, hash *string) {
				gomock.InOrder(
					q.EXPECT().SpendUnusedAuthTokens(gomock.Any(), sqlc.SpendUnusedAuthTokensParams{UserID: 7, Purpose: PurposeResetPassword}).Return(nil),
					q.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).DoAndReturn(
						func(_ context.Context, p sqlc.InsertAuthTokenParams) error {
							require.Equal(t, PurposeResetPassword, p.Purpose)
							require.Equal(t, int64(7), p.UserID)
							require.Equal(t, now.Add(time.Hour), p.ExpiresAt)
							*hash = p.TokenHash
							return nil
						}),
				)
			},
		},
		{
			name: "a spend error stops before anything is stored",
			stub: func(q *sqlc.MockQuerier, _ *string) {
				q.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantErr: "SpendUnusedAuthTokens",
		},
		{
			name: "an insert error is returned",
			stub: func(q *sqlc.MockQuerier, _ *string) {
				q.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantErr: "InsertAuthToken",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			var stored string
			tc.stub(q, &stored)

			plain, err := IssueAuthToken(context.Background(), q, PurposeResetPassword, 7, ResetTokenTTL, now)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Empty(t, plain)
				return
			}
			require.NoError(t, err)
			require.Len(t, plain, 43)
			require.Equal(t, onetimetoken.Hash(plain), stored, "only the hash of the link's token is stored")
		})
	}
}

func TestEmails_U(t *testing.T) {
	testCases := []struct {
		name string
		got  mailer.Message
		want mailer.Message
	}{
		{name: "the reset email links to the front end's reset page with the token",
			got: ResetEmail("https://app.example.com", "jane@example.com", "tok"),
			want: mailer.Message{To: "jane@example.com", Subject: "Reset your password",
				Text: "Someone asked to reset the password of your account. Open this link within 1 hour to choose a new one:\n\n" +
					"https://app.example.com/reset-password?token=tok\n\nIf it wasn't you, ignore this email; your password stays as it is."}},
		{name: "the password-changed notice",
			got: PasswordChangedEmail("jane@example.com"),
			want: mailer.Message{To: "jane@example.com", Subject: "Your password was changed",
				Text: "The password of your account was just changed and every other device was signed out. If it wasn't you, reset your password now and contact support."}},
		{name: "the verification email links to the front end's verify page with the token",
			got: VerifyEmail("https://app.example.com", "jane@example.com", "tok"),
			want: mailer.Message{To: "jane@example.com", Subject: "Verify your email",
				Text: "Open this link within 24 hours to verify your email:\n\nhttps://app.example.com/verify-email?token=tok"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.got)
		})
	}
}

func TestEmailer_Send_U(t *testing.T) {
	msg := mailer.Message{To: "jane@example.com", Subject: "Reset your password", Text: "https://app/reset-password?token=secret-token"}

	testCases := []struct {
		name     string
		mailErr  error
		wantLogs bool
	}{
		{name: "the email goes out in the background with the request id, nothing logged", mailErr: nil},
		{name: "a failed send is logged with purpose and recipient id, never the address or the link", mailErr: errors.New("smtp down"), wantLogs: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			fake := &mailer.Fake{Err: tc.mailErr}
			var gotCtx context.Context
			e := Emailer{Mailer: fake, Go: func(ctx context.Context, fn func(context.Context)) { gotCtx = ctx; fn(ctx) }}

			ctx, cancel := context.WithCancel(requestid.With(context.Background(), "req-1"))
			cancel() // the request is over; the send must not inherit its cancellation
			e.Send(ctx, PurposeResetPassword, 7, msg)

			require.Equal(t, []mailer.Message{msg}, fake.Sent())
			require.NoError(t, gotCtx.Err(), "the background send has its own context")
			require.Equal(t, "req-1", requestid.FromContext(gotCtx))
			if !tc.wantLogs {
				require.Empty(t, buf.String())
				return
			}
			var rec map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec), buf.String())
			require.Equal(t, "send email failed", rec["msg"])
			require.Equal(t, "ERROR", rec["level"])
			require.Equal(t, PurposeResetPassword, rec["purpose"])
			require.InDelta(t, 7, rec["recipient_id"], 0)
			require.Equal(t, "req-1", rec["request_id"])
			require.NotContains(t, buf.String(), "jane@example.com", "no address in the log")
			require.NotContains(t, buf.String(), "secret-token", "no link in the log")
		})
	}
}
