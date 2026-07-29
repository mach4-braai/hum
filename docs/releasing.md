# Releasing

A release is a tag. Everything else is the `Release` workflow.

## Cutting one

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs `mise run check` on Linux and macOS, then GoReleaser builds
every target, publishes the archives, the source tarball and `checksums.txt`,
and finally the `tap` job rewrites `Formula/hum.rb` in
[`mach4-braai/homebrew-tap`](https://github.com/mach4-braai/homebrew-tap) so
`brew upgrade hum` sees the new version.

A tag containing a hyphen — `v0.2.0-rc1` — is published as a GitHub prerelease
and does not reach the tap. The `tap` job is gated on the tag name for exactly
that reason: a formula pointing at a release candidate would make it the default
`brew install hum`.

Two tags released close together are serialised: the `tap` job takes a
concurrency group shared by every release, and it refuses a version the tap
already exceeds. An older tag finishing last leaves the tap alone rather than
downgrading it.

`mise run snapshot` builds the same artefacts into `dist/` without publishing
anything. Linux archives are skipped unless `HUM_RELEASE_LINUX=1` and an ALSA
cross-toolchain are present, so a snapshot on macOS covers macOS and Windows
only.

## The formula

`Formula/hum.rb` in this repository is the source of truth. The tap holds a copy
with two lines rewritten:

- `url`, to the released source tarball.
- `sha256`, taken from the release's own `checksums.txt` rather than recomputed,
  so the published checksum and the formula cannot disagree.

Everything else — the build flags, the `on_linux` dependencies, the service
block, the `test do` block — is reviewed here and travels verbatim. The `url` and
`sha256` committed here are placeholders; only the tap's copy is ever installed.

The bump creates `Formula/hum.rb` if the tap does not have it yet, so the first
release needs no manual copy, and it exits without a commit when the tap already
carries the version, so re-running a release is safe.

## The credential

A workflow's `GITHUB_TOKEN` is scoped to the repository it runs in, so nothing
here can push to the tap with it. The options are a personal access token, a
deploy key or a GitHub App. The first two are long-lived secrets that have to be
rotated on a calendar; an App's installation token is minted per job, scoped to
one repository, and revoked when the job ends. The `tap` job mints one.

Secrets are unavailable to workflows triggered from forks, and this workflow
triggers only on a tag pushed to this repository, so a pull request cannot reach
the App.

### The App

One App serves every project that publishes into the shared tap. Creating it
needs organisation-owner rights and is done once.

Organisation settings → Developer settings → GitHub Apps → New GitHub App:

| Field | Value |
|---|---|
| Name | `mach4-braai-tap-bumper` |
| Homepage URL | the tap's URL |
| Webhook | unchecked |
| Repository permissions | Contents: Read and write — nothing else |
| Installation target | Only this account |

Generate a private key, then install the App on `mach4-braai/homebrew-tap` and
nothing else: Install App → Only select repositories.

### The two values

Organisation settings → Secrets and variables → Actions, both limited to the
repositories that release:

| Kind | Name | Value |
|---|---|---|
| Variable | `TAP_APP_CLIENT_ID` | the App's client id, `Iv23…`. Not sensitive |
| Secret | `TAP_APP_PRIVATE_KEY` | the private key, whole PEM including its `BEGIN`/`END` lines |

`internal/infra` asserts that the release workflow names exactly one secret,
`TAP_APP_PRIVATE_KEY`, and reads the client id from `vars`. Renaming either fails
`mise run check` rather than a release.

Rotating the key is: generate a second private key, replace the secret, delete
the first. The App, its client id and its installation are untouched.

## Verifying a release

```sh
brew update && brew upgrade hum
hum version
```

`hum version` reports the tag, the commit and the build date stamped into the
binary. A mismatch against the release means the formula and the tag disagree —
check the tap's last commit.
