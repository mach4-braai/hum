# Tap-side formula bump

`bump-formula.yml` belongs in the `mach4-braai/homebrew-tap` repository, at
`.github/workflows/bump-formula.yml`. It is kept here because it is versioned
alongside the formula it updates, and because a workflow outside
`.github/workflows/` is inert in this repository.

## Why the bump runs in the tap rather than here

A workflow's `GITHUB_TOKEN` is scoped to the repository it runs in, so a job in
`hum` cannot push to the tap without a cross-repository credential — a personal
access token, a deploy key or a GitHub App installation token.

A personal access token and a deploy key are themselves long-lived secrets held
in a public repository, and both need rotating. A GitHub App is better than
either: the installation token it mints expires within the hour. It does not
remove the stored secret though, it changes which one — the App's private key
becomes the durable credential, and releasing then depends on an App that
someone has to own.

Inverting the direction removes the stored credential entirely. The tap reads
`hum`'s public releases, which needs no authentication beyond its own token, and
commits to itself, which that token is already allowed to do.

The cost is latency: a release is picked up by the next scheduled run rather
than the moment the tag lands. `workflow_dispatch` covers the case where that
wait is unwanted.

## Install

1. Copy `bump-formula.yml` to `.github/workflows/bump-formula.yml` in the tap.
2. Copy `Formula/hum.rb` from this repository to `Formula/hum.rb` in the tap,
   for the first release only. Every later release is bumped by the workflow.
3. Confirm the tap's Actions settings allow workflows to write contents:
   Settings → Actions → General → Workflow permissions → Read and write.

The workflow is idempotent. It compares the version in the formula's `url`
against `hum`'s latest release tag and exits without a commit when they match,
so running it on a schedule costs one API call on a day with no release.

## What it rewrites

Only the `url` and `sha256` lines. The rest of the formula is taken verbatim
from the release tag, so the build flags, the `on_linux` dependencies and the
service block stay under review in this repository rather than drifting in the
tap.

`sha256` comes from the release's own `checksums.txt` rather than being
recomputed locally, so the published checksum and the formula cannot disagree.
