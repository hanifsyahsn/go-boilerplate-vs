# Architecture decisions

`go_boilerplate_vs` is the **spearhead** for the services derived from it: an architecture decision is made,
built and tested here first, then every derivative adopts it (or records why it doesn't apply). One file per
decision, numbered, never renumbered; a reversed decision gets a new file that supersedes the old one.

Statuses: **built** = implemented in the boilerplate · **accepted** = agreed, not built here yet.

| # | Decision | Status here |
|---|---|---|
| [0001](0001-decision-log-and-propagation.md) | Decision log + propagation to derivatives | built |
| [0002](0002-baseline.md) | Baseline: what every derivative follows from the boilerplate today | built |
| [0003](0003-rate-limit.md) | Rate limit: `Limiter` interface, token bucket, optional Redis | part built (in-memory: general per-IP, auth per-IP, failed logins per email and per IP) |
| [0004](0004-csrf-origin-guard.md) | CSRF `OriginGuard` | accepted |
| [0005](0005-cleanup-worker.md) | Worker that deletes expired rows | accepted |
| [0006](0006-sessions-devices.md) | Signed-in devices list + remote sign-out | accepted |
| [0007](0007-password-and-email-flows.md) | Forgot / change password, email verification, `Mailer` | accepted |
| [0008](0008-api-activity-log.md) | API activity log | accepted |
| [0009](0009-cors.md) | CORS allowlist | accepted |
| [0010](0010-api-key.md) | API-key middleware for service-to-service calls | accepted |
| [0011](0011-ops-notifier.md) | Ops alerts through a `Notifier` interface (Telegram first) | accepted |
| [0012](0012-patterns-in-slice-patterns.md) | Recurring derivative patterns go into `slice-patterns.md` | accepted |
| [0013](0013-lint.md) | Lint: staticcheck + golangci-lint (`make lint`) | built |
| [0014](0014-after-coding-check.md) | After coding: `make check` (gofmt, go vet, lint, go test), humans and AI | built |
| [0015](0015-error-code-carries-status.md) | An error code carries its HTTP status and title (`apperror.NewCode`) | built |
| [0016](0016-piece-done-size-review-stop.md) | One piece: done checklist, size, and when code review stops | built |
| [0017](0017-race-detector-in-docker.md) | Race detector: `go test -race` inside Docker (`make test_race`) | accepted |
| [0018](0018-password-limit-in-bytes.md) | Password length is validated in bytes (`maxbytes=72`), bcrypt's limit | built |

The list of derivatives, where each decision came from and the adoption tracker are kept out of this public repo,
in a gitignored `DERIVATIVES.local.md` next to this file on the maintainer's machine.
