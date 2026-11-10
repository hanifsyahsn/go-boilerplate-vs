# 0010 — API-key middleware

Status: accepted (2026-11-10), not built. Source: implementations in derivatives.

## Decision
Service-to-service routes accept an `X-API-Key` compared in constant time; a refusal is a JSON:API 401, marked
for the access log.

## Why
The standard way for machine callers between services.
