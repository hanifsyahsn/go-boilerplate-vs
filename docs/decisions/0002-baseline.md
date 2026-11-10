# 0002 — Baseline

Status: built (everything below exists in the boilerplate as of commit 871930c)

What a derivative follows from the boilerplate today. Details live in `CLAUDE.md` and `.claude/rules/`.

| Area | Decision | Where |
|---|---|---|
| Layout | Vertical slices, one action per slice, `<domain>/shared` for shared queries | `CLAUDE.md` *Architecture* |
| HTTP | JSON:API 1.1 on both sides: media type middleware (415 / 406), `BindResource`, `ParseQuery`, JSON:API errors | `rules/jsonapi.md` |
| Router | `server.Handlers` struct; graceful shutdown | `CLAUDE.md` |
| Logging | `log/slog`, `request_id` + `user_id` on every line, `AccessLog`, detached goroutines | `rules/logging.md` |
| Auth (local users) | bcrypt; ES256 JWT with `typ`; DB-backed refresh rotation + reuse detection; trusted vs untrusted sessions; `password.CheckOrWaste` | `rules/auth.md` |
| Security | Per-IP limits on register / login, per-email login-failure limit, `TRUSTED_PROXIES` (to be reshaped by 0003) | `rules/auth.md` |
| Runtime | gin release mode unless `ENVIRONMENT=development`; container uid 10001; pagination `MaxPage` | `CLAUDE.md` *Gotchas* |
| Toolchain | go.mod `toolchain` + patch-tagged Dockerfile builder, bumped together | `CLAUDE.md` *Gotchas* |
| Scanning | `make sec` (gosec, semgrep + `.semgrep/`, govulncheck, gitleaks), local-only guard; findings fixed or `#nosec` with a reason | `Makefile`, `CLAUDE.md` |
| Lint | `make lint` (staticcheck + golangci-lint), see 0013; `make check` after every change (gofmt, vet, lint, test), see 0014 | `Makefile`, `.golangci.yml`, `staticcheck.conf`, `CLAUDE.md` |
| Tests | Table-driven, `_U` / `_I`, `RunWithDB`, container start retry, behaviour-named cases | `CLAUDE.md` *Testing conventions* |
| Docs for Claude | `.claude/rules/` path-scoped; `slice-patterns.md` independent of any slice | `CLAUDE.md` |

Doesn't apply: the auth rows for a service without local users (machine-to-machine APIs).
