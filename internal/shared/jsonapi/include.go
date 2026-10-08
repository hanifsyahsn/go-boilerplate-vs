package jsonapi

import (
	"fmt"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

// IncludeConfig declares, for one endpoint, which relationship paths a
// client may pull into "included" via ?include, and which the server expands
// by default when the client sends no ?include at all. A path may be dotted
// ("created_by.company") to reach through a relationship.
//
// The policy is opt-in with a server default:
//
//	(no ?include)        -> Default is applied
//	?include=a,b         -> exactly a,b; each MUST be in Allowed, else 400
//	?include= (empty)    -> nothing is included (relationship linkage still shown)
type IncludeConfig struct {
	Allowed []string
	Default []string
}

// ResolveIncludePolicy turns the raw ?include value into an IncludePolicy
// rooted at resourceType. present reports whether the parameter appeared in
// the URL at all. A path not in cfg.Allowed is a 400 pointing at the include
// parameter (JSON:API: an unsupported include path MUST be a 400).
func ResolveIncludePolicy(raw string, present bool, resourceType string, cfg IncludeConfig) (IncludePolicy, *apperror.AppError) {
	names := cfg.Default
	if present {
		names = nil
		if raw != "" {
			names = strings.Split(raw, ",")
		}
	}

	allowed := make(map[string]bool, len(cfg.Allowed))
	for _, a := range cfg.Allowed {
		allowed[a] = true
	}

	root := IncludePolicy{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !allowed[n] {
			return nil, apperror.New(
				apperror.CodeBadRequest,
				fmt.Sprintf("Can't include %q. Allowed values: %s.", n, strings.Join(cfg.Allowed, ", ")),
				fmt.Errorf("unsupported ?include=%q for %s (allowed: %v)", n, resourceType, cfg.Allowed),
			).WithParameter("include")
		}
		node := root
		for _, segment := range strings.Split(n, ".") {
			if _, ok := node[segment]; !ok {
				node[segment] = IncludePolicy{}
			}
			node = node[segment]
		}
	}

	if len(root) == 0 {
		return nil, nil
	}
	return IncludePolicy{resourceType: root}, nil
}
