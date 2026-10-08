package middleware

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// UserStatusReader is the one lookup the middleware needs from the DB —
// satisfied by the middleware's own sqlc Queries (see query/queries.sql).
type UserStatusReader interface {
	GetUserIsActive(ctx context.Context, id int64) (bool, error)
}

// inactiveAccountMessage is the 401 detail for a valid token whose user has
// been deactivated (or deleted) since it was issued.
const inactiveAccountMessage = "Your account is no longer active."

// AccessTokenMiddleware authenticates a request from its access token (cookie
// first, then `Authorization: Bearer`) and then checks the token's user is
// still active in the DB — a valid signature and an unexpired token aren't
// enough on their own, or a deactivated user could keep using the API until
// the token ran out. It sets the caller's user id and role in the context.
func AccessTokenMiddleware(jwt jwttoken.M, users UserStatusReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := extractToken(c)
		if err != nil {
			unauthorized(c, apperror.CodeUnauthorized.SafeToThrow(), err)
			return
		}

		claims, err := jwt.VerifyAccessToken(tokenString)
		if err != nil {
			unauthorized(c, apperror.CodeUnauthorized.SafeToThrow(),
				fmt.Errorf("jwt.VerifyAccessToken() (Access Token Middleware): %w", err))
			return
		}

		userID, err := jwttoken.SubjectID(claims)
		if err != nil {
			// defensive: VerifyAccessToken already checked sub
			unauthorized(c, apperror.CodeUnauthorized.SafeToThrow(),
				fmt.Errorf("jwttoken.SubjectID() (Access Token Middleware): %w", err))
			return
		}

		active, err := users.GetUserIsActive(c.Request.Context(), userID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			unauthorized(c, inactiveAccountMessage,
				fmt.Errorf("users.GetUserIsActive() (Access Token Middleware): user %d not found: %w", userID, err))
			return
		case err != nil:
			jsonapi.WriteError(c.Writer, apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("users.GetUserIsActive() (Access Token Middleware): %w", err))
			c.Abort()
			return
		case !active:
			unauthorized(c, inactiveAccountMessage,
				fmt.Errorf("users.GetUserIsActive() (Access Token Middleware): user %d is not active", userID))
			return
		}

		c.Set(internalsharedconstant.UserIdKey, userID)

		// A token without a role reads as "" and is treated as unprivileged
		// by RequireRole downstream.
		role, _ := claims[platformconstant.RoleKey].(string)
		c.Set(internalsharedconstant.UserRoleKey, role)

		c.Next()
	}
}

// extractToken reads the access token from its cookie, falling back to an
// `Authorization: Bearer <token>` header.
func extractToken(c *gin.Context) (string, error) {
	if token, err := c.Cookie(internalsharedconstant.AccessTokenCookieKey); err == nil && token != "" {
		return token, nil
	}

	authHeader := c.GetHeader(platformconstant.AuthorizationHeaderKey)
	if strings.TrimSpace(authHeader) == "" {
		return "", errors.New("extractToken() (Access Token Middleware): no access token in the cookie or the authorization header")
	}

	fields := strings.Fields(authHeader)
	if len(fields) != 2 || fields[0] != platformconstant.AuthorizationBearerKey {
		return "", errors.New("extractToken() (Access Token Middleware): invalid authorization header format")
	}

	return fields[1], nil
}

func unauthorized(c *gin.Context, message string, err error) {
	jsonapi.WriteError(c.Writer, apperror.CodeUnauthorized, message, err)
	c.Abort()
}
