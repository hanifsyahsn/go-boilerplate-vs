---
paths:
  - "cmd/api/**"
  - "internal/platform/logging/**"
  - "internal/platform/middleware/**"
  - "internal/platform/server/**"
  - "internal/shared/requestid/**"
  - "internal/shared/requestuser/**"
  - "internal/shared/utility/goroutine*"
  - "internal/shared/jsonapi/**"
  - "internal/shared/handlerutil/**"
  - "internal/features/**/service.go"
---
# Logging (log/slog)

Moved verbatim from CLAUDE.md on 2026-11-04. Related rules: jsonapi.md (*Request flow*: `AccessLog`, how
`WriteErrorsContext` logs). What applies everywhere is in the root CLAUDE.md.

### Logging

- `log/slog` only. `logging.Setup(conf.LogFormat, conf.LogLevel)` in `main` makes the default logger (stdout;
  `LOG_FORMAT` text / json, `LOG_LEVEL`); its handler adds `request_id` (`requestid.FromContext`) and the
  caller's `user_id` (`requestuser.FromContext`) to every record; both also read a `*gin.Context` (services get
  one as their ctx).
- **Use the `*Context` variant whenever a ctx is in scope** — that's where `request_id` comes from. Short
  lowercase message + key/value attributes, never a formatted string:
  `slog.ErrorContext(ctx, "notify: get note failed", "note_id", id, "err", err)`. **`user_id` always means the
  caller and comes from ctx** — never add it by hand on the request path; name another user for what it is
  (`target_user_id`, `recipient_id`). Background workers use `worker run failed` / `worker job failed` /
  `worker stopped` with a `worker` attribute, and (no request ctx) may log `user_id` for the user they process.
- Writing an error from a handler / middleware: pass the **request's** context —
  `jsonapi.WriteErrorsContext(handlerutil.RequestContext(c), c.Writer, ...)` (`handlerutil.WriteError`,
  `ParseQuery`, `ParamID` and `AccessTokenMiddleware` do). Not the `*gin.Context`: its `Err()` is always nil, so
  a cancellation would read as a server error. `RequestContext` falls back to `context.Background()` when
  `c.Request` is nil (`gin.CreateTestContext`).
- Startup failures: `logging.Fatal("load config failed", "err", err)`. A `TestMain` may use `log.Fatalf`.
- A goroutine that outlives the request: `utility.SafeGoContext(requestid.Detach(ctx), fn)` — `Detach` keeps
  the request id and the caller, so its logs (and a recovered panic) carry both. **Never `context.WithoutCancel(c)` on a `*gin.Context`**: gin reuses it for the next
  request. Anything async (an emailer, …) takes a ctx for the same reason.
- Tests that assert on logs swap the default: `prev := slog.Default(); slog.SetDefault(l); defer
  slog.SetDefault(prev)` with `l, _ := logging.New(&buf, logging.FormatJSON, "debug")`.
