# 0016 — One piece: the done checklist, its size, and when code review stops

Status: built (2026-11-10)

Extends 0014 (`make check` after coding).

## Decision

### 1. A piece is done when, in this order
1. **`make check`** passes (0014: gofmt, go vet, staticcheck + golangci-lint, the full `go test ./...`).
2. **`docs/API.md`** is updated when the piece changes the API (the frontend reads it).
3. **Every agreed behaviour** (each decision, each edge case) has a test that **asserts the outcome** — state,
   response, side effects — named after the behaviour; the author reports the behaviour → test mapping.
4. **100% coverage of the new, non-generated code** (`make test_coverage`, then `go tool cover -func coverage.out`
   for the touched files). Coverage is supporting evidence; the mapping in 3 is the answer to "is it safe?".

Only then does the piece go to code review (`/code-review`, or a human reviewer).

### 2. Size of a piece: one complete unit of work
- One piece = one endpoint, one small feature, or one complete bug fix, each with its tests. Each piece is its
  own commit. A ticket is planned up front as a list of pieces.
- Each piece stands on its own: it builds and its tests pass.
- Not smaller: every review has a base cost, and ten 5-line reviews cost more than one review of the same task.
  Not bigger: a whole ticket reviewed as one diff at the end gets a blunter review and more rounds.

### 3. When code review stops
An AI reviewer always finds something, so review ends by rule, not by itself.
- **Fix now** only:
  a. a bug that can happen in normal use (wrong result, wrong access, broken data);
  b. a security hole;
  c. a break of the agreed API contract (`docs/API.md`).
- **Everything else goes to the backlog**, without a long discussion: millisecond-wide races, refactors, small
  performance gains, and findings already declined before. The backlog is the repo's ticket tracker; in this
  public boilerplate it is a gitignored `docs/review-backlog.local.md` (known weaknesses aren't published).
  It is read before a review, so a declined finding isn't argued again.
- **One review round per piece.** A second round only when the first found an a / b / c finding, and it checks
  only those fixes.

## Why
A multi-slice task reviewed once at the end as one big diff took four review rounds: the first found real bugs,
the second a security hole, the last two only tiny races and findings repeated from earlier rounds (one came up
in all four). Most of the cost was in the rounds that changed nothing that mattered. Right-sized pieces keep each
review short and sharp; a stop rule keeps it from running on; the done checklist means the reviewer isn't the
one finding missing tests, docs or a red lint.

## Adopting it in a derivative
Needs 0013 and 0014 first. Until a derivative's lint is clean, step 1.1 also names every lint finding that was
already there before the piece (not introduced by it). Copy the *After coding* section of `CLAUDE.md` and the
`README.md` paragraph; point the backlog at that repo's ticket tracker. A repo whose API has no frontend reader
may mark 1.2 as not applicable, in its `CLAUDE.md`.
