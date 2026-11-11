---
paths:
  - "cmd/api/**"
  - "internal/platform/logging/**"
  - "internal/features/activitylog/**"
  - "internal/features/cleanup/**"
  - "internal/platform/notifier/**"
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
- **Work that must finish before shutdown** (a worker, an email send, the activity recorder) runs in main's
  `utility.Group` (`bg.Go(ctx, fn)` — `SafeGoContext` underneath), never a bare `SafeGoContext`. `main` cancels
  `bgCtx` only **after** `srv.Run` returned (requests drained), then `bg.Wait()`, then the DB pool closes — so the
  last requests' work is flushed, not lost. A service that starts such work takes a `utility.GoFunc` (main passes
  `bg.Go`; tests pass one that runs `fn` inline) and hands it `requestid.Detach(ctx)`. A worker returns when its
  ctx is done, after flushing.
- A goroutine that outlives the request: `utility.SafeGoContext(requestid.Detach(ctx), fn)` — `Detach` keeps
  the request id and the caller, so its logs (and a recovered panic) carry both. **Never `context.WithoutCancel(c)` on a `*gin.Context`**: gin reuses it for the next
  request. Anything async (an emailer, …) takes a ctx for the same reason.
- Tests that assert on logs swap the default: `prev := slog.Default(); slog.SetDefault(l); defer
  slog.SetDefault(prev)` with `l, _ := logging.New(&buf, logging.FormatJSON, "debug")`.

### Activity log

- **API activity log** (decision 0008, migration 000008, table `api_activity_logs`, package
  `internal/features/activitylog`): one row per request — when, `request_id`, user and session (NULL = not
  signed in; no FK on purpose), method, the matched **route** pattern (`c.FullPath()`, empty for 404 / 405),
  path **without** the query string, status, latency, response size, client IP, User-Agent. **No body or query
  string** (they can hold passwords and tokens). `middleware.ActivityLog` runs on the engine right after
  `AccessLog`, outside recovery (404 / 405 / panics recorded too), skips `/healthz`, cuts values to the column
  sizes in runes.
- `activitylog.Recorder`: `Record` never blocks (a 4096 queue; full → dropped, counted, logged as `activity log:
  queue full, rows dropped` at the next write); `Run` (in main's background group) writes batches of 200 or every
  2 s in **one** `INSERT … SELECT unnest(…)` per batch; a failed write → `activity log: write failed` with `rows`,
  dropped (no retry). On shutdown it drains what is queued and writes it on `context.WithoutCancel` + 5 s —
  main cancels it only after the server drained its requests (*Logging* above). Rows older than
  `ACTIVITY_LOG_RETENTION` (default `17520h`, `0` = forever) are deleted by the cleanup worker (decision 0005).
- Tests: `TestActivityLog_U`, `TestRecorder_U` / `_QueueFull_U` / `_Drain_U`, `TestRecorder_ShutdownFlush_I`
  (500 rows, cancel, all 500 in the table), `-race` in Docker.

### Ops alerts

- **`internal/platform/notifier`** (decision 0011): `Notifier.Notify(ctx, Alert{Level, Title, Text, RequestID})`
  never blocks and never errors. `NewTelegram(token, chatID, environment, service)` when `TELEGRAM_BOT_TOKEN` +
  `TELEGRAM_CHAT_ID` are set (both or neither), else `NewNoop()`. Telegram: a 100-alert queue sent by `Run` in
  main's background group (full → dropped, warn `notifier: queue full, alert dropped`); **one alert per `Title`
  per 5 min**, the held-back ones counted into the next ("(N more since)"); plain text `"[env] service — LEVEL:
  Title\nText"` (+ `request_id`), no link preview, 5 s timeout. **The bot token never reaches a log**: a send
  failure is `notifier: send failed` with `status` and the error with the token replaced by `***` (a
  `*url.Error` carries the URL, which carries the token).
- What alerts: every recovered panic (`utility.SetPanicHook`, called by `SafeGoContext`'s recover and the
  router's recovery — main sets it to `Alert{error, "panic", …, request id}`), `cleanup failed: <table>`,
  `activity log: write failed` (the worker and the recorder take it through `SetNotifier`). A `Title` is short
  and stable (it is the dedupe key); a `Text` never holds a body, token, password or email address.
