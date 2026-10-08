package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

const feature = "Admin User service"

const loadErrorMessage = "We couldn't load the users. Please try again, and contact support if the problem continues."

type S interface {
	// List returns one page of the user directory, in req.Sort order.
	List(ctx context.Context, req ListRequest) ([]UserResource, pagination.Meta, error)
	// Update applies a PATCH to a user and returns it: is_active activates /
	// deactivates (nil changes nothing). An admin can't deactivate
	// themselves (409) — that would lock them out on their next request.
	Update(ctx context.Context, userID int64, req UpdateRequest, actorID int64) (UserResource, error)
}

type service struct {
	querier sqlc.Querier
}

func NewService(querier sqlc.Querier) S {
	return &service{querier: querier}
}

func (s *service) List(ctx context.Context, req ListRequest) ([]UserResource, pagination.Meta, error) {
	page, pageSize, limit, offset := req.Resolve()
	q := utility.NullString(strings.TrimSpace(req.Q))
	var isActive sql.NullBool
	if req.IsActive != nil {
		isActive = sql.NullBool{Bool: *req.IsActive, Valid: true}
	}

	rows, err := s.querier.ListUsers(ctx, sqlc.ListUsersParams{Q: q, IsActive: isActive, Sort: req.Sort, Limit: limit, Offset: offset})
	if err != nil {
		return nil, pagination.Meta{}, apperror.New(apperror.CodeInternal, loadErrorMessage,
			fmt.Errorf("querier.ListUsers() (%s): %w", feature, err))
	}

	total, err := s.querier.CountUsers(ctx, sqlc.CountUsersParams{Q: q, IsActive: isActive})
	if err != nil {
		return nil, pagination.Meta{}, apperror.New(apperror.CodeInternal, loadErrorMessage,
			fmt.Errorf("querier.CountUsers() (%s): %w", feature, err))
	}

	items := make([]UserResource, 0, len(rows))
	for _, r := range rows {
		items = append(items, fromRow(sqlc.GetUserRow(r)))
	}

	return items, pagination.NewMeta(page, pageSize, total), nil
}

func (s *service) Update(ctx context.Context, userID int64, req UpdateRequest, actorID int64) (UserResource, error) {
	if req.IsActive == nil {
		// Nothing to change: answer with the user as it stands.
		r, err := s.querier.GetUser(ctx, userID)
		if err != nil {
			return UserResource{}, userError(err, "GetUser", userID, "We couldn't load the user.")
		}
		return fromRow(r), nil
	}

	if userID == actorID && !*req.IsActive {
		return UserResource{}, apperror.New(apperror.CodeConflict, "You can't deactivate your own account.",
			fmt.Errorf("(%s): admin %d tried to deactivate themselves", feature, actorID))
	}

	r, err := s.querier.SetUserActive(ctx, sqlc.SetUserActiveParams{ID: userID, IsActive: *req.IsActive})
	if err != nil {
		return UserResource{}, userError(err, "SetUserActive", userID, "We couldn't update the user.")
	}
	return fromRow(sqlc.GetUserRow(r)), nil
}

// fromRow builds the response resource of one user. The list / update rows
// are distinct generated types with the same columns, so they convert to
// GetUserRow.
func fromRow(r sqlc.GetUserRow) UserResource {
	return UserResource{
		ID:        strconv.FormatInt(r.ID, 10),
		Name:      r.Name,
		Email:     r.Email,
		Role:      r.Role,
		IsActive:  r.IsActive,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// userError maps a single-user query error: no row is a 404, anything else
// a 500 worded with failed.
func userError(err error, query string, userID int64, failed string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.New(apperror.CodeNotFound, "That user was not found.",
			fmt.Errorf("querier.%s(%d) (%s): %w", query, userID, feature, err))
	}
	return apperror.New(apperror.CodeInternal,
		failed+" Please try again, and contact support if the problem continues.",
		fmt.Errorf("querier.%s(%d) (%s): %w", query, userID, feature, err))
}
