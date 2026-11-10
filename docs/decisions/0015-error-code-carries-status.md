# 0015 — An error code carries its HTTP status and title

Status: built (2026-11-10)

## Decision
1. `apperror.Code` is a struct `{id, status, title}` with unexported fields, made only with
   `apperror.NewCode(id, status)`; the title defaults to `http.StatusText(status)`, and `.WithTitle(t)` overrides it
   (today only `token_expired` → 401 "Token Expired").
2. A domain declares its codes **in its own package**, as package-level vars, next to the logic that returns them:
   `var CodeNoteArchived = apperror.NewCode("note_archived", http.StatusConflict)`. `internal/shared` doesn't change
   when a domain adds a code.
3. `Code.Status()`, `Code.Title()`, `Code.String()` (the JSON:API `code`) read the value itself. The two switches
   (`MapToHTTPStatus`, `Code.Title`) are gone, and so is `MapToHTTPStatus` (use `code.Status()`).
4. `NewCode` panics on an empty id or a status outside 400–599 (and `WithTitle` on an empty title): codes live in
   package vars, so a bad one stops the program at startup. The zero `Code{}` is an unknown error: 500 /
   "Internal Server Error"; `IsZero()` tests for it (tests use the zero code for "no error expected").
5. No global registry and no `init()` registration. Codes stay comparable (`==`, `switch`).
6. Every existing code keeps its id, status and title; a test pins all of them.

## Why
With a string code and two switches, a code missing from either switch silently answered 500 / "Internal Server
Error" — even for a business conflict — and every domain edited the same shared file. The only guard was a
convention (add a test case per switch). Carrying the status in the value makes a code without one impossible to
write, removes the shared switches, and needs no map or init order. A title derived only from the status was
rejected because `token_expired` has its own.

Rejected: a registry map read by both functions (still global state, still a runtime lookup); per-domain error
types implementing `Status()` (a type per error for the same guarantee); code generation (a tool for a one-line
declaration).

## Adopting it in a derivative
1. Replace `type Code string` + the constants + `Title()` / `MapToHTTPStatus` switches with the boilerplate's
   `Code` struct, `NewCode`, `WithTitle`, `String`, `IsZero`, `Status`, `Title` and the generic codes.
2. Move each domain code out of `apperror` into its domain package as `NewCode(...)`, with the status it had in
   the switch (a title only if it differed from the status text).
3. `MapToHTTPStatus(c)` → `c.Status()`; `string(c)` → `c.String()`; `wantCode != ""` → `!wantCode.IsZero()`;
   `Code("x")` in tests → `NewCode("x", status)` or `Code{}`.
4. Keep (or add) one test pinning every existing code's id, status and title, so the move is proven unchanged.
5. `make check`.
