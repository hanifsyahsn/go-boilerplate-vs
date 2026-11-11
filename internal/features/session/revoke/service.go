package revoke

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/revoke/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Revoke Session service"

type S interface {
	// Revoke signs one of the caller's live sessions out
	// (signed_out_remotely): its refresh tokens and its access tokens stop
	// working at once. An id that isn't a UUID, someone else's session or
	// one that isn't live is a 404 (someone else's reads as missing).
	Revoke(ctx context.Context, userID int64, sessionID string) error
}

type service struct {
	store StoreI
}

func NewService(store StoreI) S {
	return &service{store: store}
}

func notFound(err error) error {
	return apperror.New(apperror.CodeNotFound, shared.NotFoundMessage, err)
}

func (s *service) Revoke(ctx context.Context, userID int64, sessionID string) error {
	familyID, err := uuid.Parse(sessionID)
	if err != nil {
		return notFound(fmt.Errorf("uuid.Parse(%q) (%s): %w", sessionID, feature, err))
	}

	return s.store.RunInTx(ctx, func(q Querier) error {
		if _, err := q.LockLiveSession(ctx, sqlc.LockLiveSessionParams{FamilyID: familyID, UserID: userID}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return notFound(fmt.Errorf("LockLiveSession(%s, %d) (%s): not the caller's or not live: %w", familyID, userID, feature, err))
			}
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("LockLiveSession(%s) (%s): %w", familyID, feature, err))
		}
		if _, err := q.RevokeSession(ctx, authsharedsqlc.RevokeSessionParams{
			FamilyID: familyID, Reason: authshared.RevokeReasonSignedOutRemotely,
		}); err != nil {
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("RevokeSession(%s) (%s): %w", familyID, feature, err))
		}
		return nil
	})
}
