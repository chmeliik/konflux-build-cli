# Sigstore stack for integration tests

`integration_tests/framework/sigstore_stack.go` starts a local Sigstore
deployment so that signing tests can run without network access to the public
Sigstore instance. It runs these containers on the host network:

| Service              | Port       | Purpose                                 |
| -------------------- | ---------- | --------------------------------------- |
| MySQL                | 3306       | Trillian storage                        |
| Trillian log server  | 8090, 8091 | Merkle tree backing Rekor               |
| Trillian log signer  | 8092, 8093 | Sequences log entries                   |
| Redis                | 6379       | Rekor search index                      |
| Rekor                | 3000       | Transparency log                        |
| Dex                  | 8888       | OIDC provider                           |
| Fulcio               | 5555, 5554 | Issues short-lived signing certificates |

The stack covers the happy path of keyless signing: `cosign sign`,
`cosign attest`, `cosign attach sbom`, and the matching verify commands,
including upload to and inclusion in the transparency log.

It is deliberately much weaker than a real deployment. The sections below
describe what the tests cannot tell you, so that you don't read more into a
passing test than it proves.

## No TUF root of trust

A real deployment distributes its trust material through a TUF repository.
Clients bootstrap with `cosign initialize --root <root.json> --mirror <url>`,
and from then on pick up key rotations, expiry, and threshold signatures from
signed TUF metadata.

The Sigstore scaffolding TUF server stores its repository in a Kubernetes
Secret and refuses to start outside a cluster, so the stack does not run it.
Instead, the stack generates a trusted root and a signing config with
`cosign trusted-root create` and `cosign signing-config create`, mounts both
files into the test container, and the tests pass them via `--trusted-root`
and `--signing-config`.

As a result the tests exercise none of: TUF bootstrapping, metadata expiry,
key rotation, threshold signing, or the client-side TUF cache. A regression
that only shows up when trust material arrives over TUF will pass here.

## No certificate transparency

Real Fulcio submits every certificate it issues to a CT log and embeds the
returned SCT in the certificate. Clients verify that SCT.

The stack runs Fulcio with an empty `--ct-log-url`, so certificates carry no
SCT and the verify commands need `--insecure-ignore-sct`. The entire CT
submission and SCT verification path is untested.

## Weak certificate authority

Fulcio runs as a `fileca` backed by a self-signed P-256 key that the framework
generates with `openssl` at startup. The key is world-readable on disk,
because Fulcio runs as a non-root user that does not own the file.

There is no intermediate CA — the root signs leaf certificates directly. A
real deployment keeps the root in a KMS or HSM and issues through an
intermediate.

## Mock identity

Dex runs with the `mockCallback` connector, which approves any authorization
request without credentials and always returns the same identity. The
framework scripts the OAuth2 authorization code flow from the host and passes
the resulting token to cosign via `SIGSTORE_ID_TOKEN`.

Two consequences:

- The tests cannot cover identity-based policy. They assert
  `--certificate-identity-regexp=.*`, which accepts any signer.
- They do not exercise how cosign obtains a token in production. In a Tekton
  task cosign uses ambient credentials from a projected Kubernetes service
  account token; nothing here resembles that.

## Ephemeral, single-instance transparency log

- Rekor signs checkpoints with `--rekor_server.signer=memory`, so its log
  signing key is generated fresh on every run. A real deployment uses a
  KMS-backed signer.
- MySQL has no volume, so the log starts empty on every run. Nothing tests
  behaviour against a log with history.
- Rekor creates its Trillian tree implicitly at startup, because it is given
  no `--trillian_log_server.tlog_id`. There is one shard and no sharding
  config.
- The Trillian log signer runs with `--force_master` as a single replica, so
  there is no mastership election.
- Attestation storage is disabled, so `cosign attest` stores the attestation
  in the registry only.

## Missing components

The stack has no timestamp authority, so signing config contains no TSA and
`cosign verify --use-signed-timestamps` is untestable. It also has no policy
enforcement — nothing equivalent to policy-controller or Enterprise Contract
checks the signatures the tests produce.

## Operational limits

All services bind fixed ports on the host network rather than talking over
cluster DNS. So:

- Only one stack can run per machine, and tests using it cannot run in
  parallel.
- Ports collide with anything else already listening on the host. Fulcio's
  metrics port is moved off the default 2112 because Rekor claims it first.
- Everything speaks plain HTTP. A real deployment serves HTTPS, so TLS
  verification and certificate handling are untested.

## References

See the [integration tests design](integration-tests.md) doc for the general
structure of integration tests.

| Description                      | Link                                     |
| -------------------------------- | ---------------------------------------- |
| Sigstore scaffolding, the Kubernetes deployment the images come from | https://github.com/sigstore/scaffolding |
| Rekor's own docker-compose, which the Trillian and Rekor setup follows | https://github.com/sigstore/rekor/blob/main/docker-compose.yml |
