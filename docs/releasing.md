# Releasing

A release is a tag, then a decision. The tag builds a draft; publishing the draft
ships it.

## Cutting one

```sh
git tag v0.1.3
git push origin v0.1.3
```

The `Release` workflow runs `mise run check` on Linux and macOS, then GoReleaser
builds every target and attaches the archives, the source tarball and
`checksums.txt` to a **draft** release. Nothing is public and nothing is frozen.

A run that dies half way can be run again: `replace_existing_draft` makes GoReleaser
delete the draft and recreate it, so a retry cannot leave a second draft for the tag
or a mix of assets from two builds.

## Publishing one

Review the draft's assets, then publish it:

```sh
gh release edit v0.1.3 --draft=false --latest
```

Publishing is what makes the release immutable — its assets and its tag can never
change again, and GitHub mints a release attestation binding the tag, the commit and
the assets. It is also what ships: the `Promote` workflow listens for the
`published` event and rewrites `Formula/hum.rb` in
[`mach4-braai/homebrew-tap`](https://github.com/mach4-braai/homebrew-tap), so
`brew upgrade hum` sees the new version.

Publish with `--prerelease` instead and the tap is left alone: the promote job
excludes prereleases, and separately excludes any tag containing a hyphen, because a
formula pointing at `v0.2.0-rc1` would make a candidate the default
`brew install hum`.

`promote.yml` subscribes to `published` alone, the one activity GitHub documents as
covering publication from a draft — `prereleased` is documented not to fire for one,
and adding `released` would risk two deliveries for a single publication. It checks
out the released tag rather than the default branch, so a release published weeks
later still uses the formula reviewed at that tag. Publishes are serialised by a
shared concurrency group, and the bump refuses a version the tap already exceeds, so
publishing an old draft cannot downgrade the tap.

Never reuse a tag. Deleting an immutable release lets you delete its tag, but the
name is burned permanently — cut the next version instead.

`mise run snapshot` builds the same artefacts into `dist/` without publishing
anything. Linux archives are skipped unless `HUM_RELEASE_LINUX=1` and an ALSA
cross-toolchain are present, so a snapshot on macOS covers macOS and Windows
only.

The `aarch64` cross-compiler and the ALSA headers are restored from a cached deb
archive rather than downloaded. They are 62 MB, and the Ubuntu mirror served them
at 49 kB/s once — twenty-one minutes, for a build that takes fifty seconds.

The cache is written by a `packages` job on every `master` push, not by the release
itself: a run on a tag can restore only its own ref and the default branch, so an
archive saved under `v0.1.1` would be invisible to `v0.1.2`. Both use
`.github/actions/linux-packages`, so the set that is warmed is the set that is
wanted. Issue #39 deletes all of it when oto ships a stable CGO-free Linux driver.

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

`homebrew-tapper`, owned by the organisation, installed on
`mach4-braai/homebrew-tap` and nothing else, holding `contents: write` and
`metadata: read` and nothing else. One App serves every project that publishes
into the shared tap.

The tap's `default-branch` ruleset requires pull requests, so the App is also a
bypass actor on it. Without that the bump fails at the push with "Changes must be
made through a pull request", and no assertion here can catch it: the setting
lives in the tap.

Recreating it: Organisation settings → Developer settings → GitHub Apps → New
GitHub App, with the webhook unchecked, *Repository permissions → Contents: Read
and write*, "Only this account", a generated private key, and *Install App → Only
select repositories* pointing at the tap.

### The two values, and where they live

They are read by the workflow that runs, which is this repository's. Hold them as
organisation secrets and variables limited to the repositories that release —
`hum` today — or as repository entries on `hum` itself. The tap is not one of
them.

Where they are today: `TAP_APP_CLIENT_ID` is a repository variable on `hum`, and
`TAP_APP_PRIVATE_KEY` an organisation secret granted to `hum`. Moving the client
id to an organisation variable is the tidier home once a second project releases
into the tap.

| Kind | Name | Value |
|---|---|---|
| Variable | `TAP_APP_CLIENT_ID` | the App's client id: `gh api /apps/homebrew-tapper --jq .client_id` |
| Secret | `TAP_APP_PRIVATE_KEY` | the private key, whole PEM including its `BEGIN`/`END` lines |

The client id is a **variable** because it is public — that `gh api` call needs no
authentication. Holding it as a secret masks it in the logs and reads as though
the release depended on two credentials when it depends on one.

The tap holds no secret, no variable and no workflow. What it holds is the App's
installation, which is what turns the private key into write access to it. The
credential lives with the pusher; the permission lives with the pushed-to.

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
