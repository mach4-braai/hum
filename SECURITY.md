# Security

## Reporting a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/mach4-braai/hum/security/advisories/new).
Do not open a public issue for a security matter.

If that form is not available to you, open an ordinary issue saying only that you
have a security report and asking for a private channel. Put no details in it —
not the version, not the symptom. A maintainer will open the private advisory and
invite you.

There is no SLA. This is a personal project maintained on personal time.

## Scope

`hum` is a local-first auditory display daemon. Its attack surface is narrow by design:

- `humd` listens on a Unix domain socket, `~/.hum/humd.sock` by default —
  `$HUM_HOME` moves the directory and `$HUM_SOCKET` overrides the path outright.
  The socket is `chmod`ed to `0600` after bind and its parent directory is created
  `0700`, and that file permission is the authorisation boundary: only the owner
  can connect.
- The socket speaks a documented, line-delimited JSON protocol described in
  [`docs/protocol.md`](docs/protocol.md). There is no authentication layer
  because none is needed: the OS enforces it.
- There is no network listener. `humd` does not bind TCP, UDP, or any interface
  reachable off the local machine.

On Windows the socket's parent directory is not access-restricted because
`EnsureRuntimeDir` asks for `0700` and Windows ignores it. This is a known gap,
documented in `README.md` and in `AGENTS.md`.

A finding that exploits the socket to affect another user's process on the same
machine is in scope. A finding that assumes the attacker already controls the
user's session is not.

## Supported versions

The latest release gets fixes. There is no LTS, no backport policy, and no
supported older branch.

## Verifiability

The release path is designed to be auditable:

- Releases are draft-first and immutable. Once published, assets and the tag are
  frozen. A deleted release frees the tag for deletion but the name can never be
  reused.
- Every GitHub Actions action is pinned to a full commit SHA. The SHA and the
  corresponding version tag are recorded together in `internal/infra` so a bare
  SHA is never opaque.
- GoReleaser attaches an SBOM and build provenance to every release.
