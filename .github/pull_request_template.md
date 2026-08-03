## What this does

<!-- One sentence. The commit message body carries the detail. -->

## Gates run

- [ ] `mise run check`
- [ ] `mise run coverage`
- [ ] `mise run e2e` (skip on Windows; note if skipped and why)
- [ ] `mise run vuln`

## Checklist

- [ ] No comments added to Go code
- [ ] Any new code path is exercised by a test (the per-block scan will fail otherwise)
- [ ] `go.mod` still has exactly two dependencies, or the PR body explains why a third is justified
- [ ] Does `AGENTS.md` need a new trap recorded?
