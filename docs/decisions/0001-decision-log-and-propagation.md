# 0001 — Decision log and propagation to derivatives

Status: built (2026-11-10)

## Decision
- Architecture decisions live here, one numbered file each, listed in `README.md`.
- The boilerplate leads: a decision is built and tested here first, then each derivative adopts it.
- **Every global enhancement of the boilerplate** (anything a derivative would also want: security, auth, logging,
  JSON:API, tooling, test conventions, slice patterns) gets a decision file and a tracker row, and the maintainer
  is reminded to tell the derivatives about it.
- Per decision, a derivative is: adopted / not yet / doesn't apply + reason. The tracker is private (gitignored
  `DERIVATIVES.local.md`); a derivative also records it in its own `CLAUDE.md` once it adopts.

## Why
The services derived from this boilerplate drifted apart (2026-11-10 audit). Without a log nobody in a
derivative knows a new decision exists.
