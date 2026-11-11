package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// APIKeyHeader carries a machine caller's key.
const APIKeyHeader = "X-API-Key"

// APIClientKey is the gin key holding the name of the API client a request
// authenticated as (AccessLog adds it to its record).
const APIClientKey = "api_client"

// APIKeyRequiredMessage is the 401 detail of a missing or wrong key.
const APIKeyRequiredMessage = "A valid API key is required."

type apiKey struct {
	name string
	hash [sha256.Size]byte
}

// APIKey authenticates a machine caller (another service) by the X-API-Key
// header against keys (API_KEYS, name → key). Mount it on a group of routes
// meant for machines only; the boilerplate has none (decision 0010).
//
// The given key's SHA-256 is compared with every configured key's in
// constant time, over all keys with no early exit, so neither the match nor
// its position leaks through timing. No match → 401 on source.header
// X-API-Key; a match sets the gin key api_client to the client's name.
func APIKey(keys map[string]string) gin.HandlerFunc {
	hashed := make([]apiKey, 0, len(keys))
	for name, key := range keys {
		hashed = append(hashed, apiKey{name: name, hash: sha256.Sum256([]byte(key))})
	}
	return func(c *gin.Context) {
		given := sha256.Sum256([]byte(c.GetHeader(APIKeyHeader)))
		matched := ""
		for _, k := range hashed {
			if subtle.ConstantTimeCompare(given[:], k.hash[:]) == 1 {
				matched = k.name
			}
		}
		if matched == "" || c.GetHeader(APIKeyHeader) == "" {
			jsonapi.WriteErrorsContext(handlerutil.RequestContext(c), c.Writer, apperror.New(apperror.CodeUnauthorized,
				APIKeyRequiredMessage, errors.New("APIKey: no or unknown X-API-Key")).WithHeader(APIKeyHeader))
			c.Abort()
			return
		}
		c.Set(APIClientKey, matched)
		c.Next()
	}
}
