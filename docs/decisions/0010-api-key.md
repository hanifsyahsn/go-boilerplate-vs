# 0010 — API-key middleware

Status: built (2026-11-11) — the middleware and `API_KEYS`; no route of the boilerplate uses it.

## Decision
Service-to-service routes accept an `X-API-Key` compared in constant time; a refusal is a JSON:API 401, marked
for the access log.

## Why
The standard way for machine callers between services.

## As built
- `API_KEYS` = `name:key,name:key` (keys ≥ 32 characters, names unique; config errors never echo a key).
- `middleware.APIKey(keys)`: `X-API-Key`, SHA-256 compared in constant time over every key; 401 "A valid API key
  is required." on `source.header` `X-API-Key`; gin key `api_client` = the name, which the access log records.
- Not mounted in the boilerplate (no machine callers); a derivative mounts it on its machine-only group.
