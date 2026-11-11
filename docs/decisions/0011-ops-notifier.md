# 0011 — Ops notifier

Status: built (2026-11-11). Source: implementations in derivatives.

## Decision
A `Notifier` interface for operational alerts (background failures, panics), Telegram as the first
implementation, a no-op when unconfigured. Tokens from env only.

## As built
- `internal/platform/notifier`: `Notifier` (never blocks, never errors), `NewTelegram` (async queue of 100 sent by
  `Run` in the background group, one alert per title per 5 min with "(N more since)", plain text, 5 s timeout,
  the bot token redacted from every logged error) and `NewNoop`.
- `TELEGRAM_BOT_TOKEN` + `TELEGRAM_CHAT_ID`, both or neither.
- Alerts: every recovered panic (request or background goroutine, through `utility.SetPanicHook`), a cleanup
  table's failure, a failed activity-log write.
