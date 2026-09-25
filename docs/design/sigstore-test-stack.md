# Local Sigstore stack for keyless-signing tests

## Context

Konflux-build-cli is getting subcommands that do keyless signing of container images.
These replace most of the [current Bash code in the Buildah task][buildah-keyless-code].

To test the subcommands, our test framework needs to support the following:

1. Serve a custom TUF root for use with `cosign initialize`

    ```sh
    cosign initialize --root "${TUF_URL}/root.json"
    ```

2. Provide an OIDC token. The Buildah task uses a token injected automatically by Kubernetes:

    ```sh
    SIGSTORE_ID_TOKEN=$(cat /var/run/sigstore/cosign/oidc-token)
    ```

   In our tests, it doesn't matter where the token comes from, but does need to be valid for signing.

3. Provide the services necessary for the `cosign sign` operation:

    ```sh
    cosign sign \
      --rekor-url="${REKOR_URL}" \
      --fulcio-url="${FULCIO_URL}" \
      --oidc-issuer="${OIDC_ISSUER_URL}"
    ```

   That is:
   * Rekor, a transparency log (and its dependencies)
   * Fulcio, a certificate authority that issues short-lived signing certs tied to an OIDC identity
   * An OIDC provider

## Decision

Deploy a complete local Sigstore stack based on [sigstore/scaffolding].
Don't use a Kind Kubernetes cluster, deploy it as a single Podman pod.

Reasons to use a Podman pod:

* Keeps the whole stack a single unit which can be started, stopped and inspected together.
* Makes it easy to expose the host-facing ports while keeping other ports pod-internal.
* Is far lighter and easier to manage than a Kind cluster.
* Can be defined declaratively.

Note that this ties the keyless signing tests to Podman, they won't work with Docker.
That's an acceptable tradeoff.

### Details

The sigstore stack is defined as a Kubernetes Pod in
[sigstore-stack.yaml](../../integration_tests/framework/sigstore-stack.yaml).
The test framework ([sigstore\_stack.go](../../integration_tests/framework/sigstore_stack.go))
deploys the stack as a Podman pod using `podman kube play`.

#### Components

| Container                             | Role                                          | Host-facing |
| ---                                   | ---                                           | ---         |
| `tuf`                                 | serves the trust root for `cosign initialize` | ✔️          |
| `fulcio`                              | certificate authority                         | ✔️          |
| `ctfe`                                | Fulcio's certificate transparency log         |             |
| `dex`                                 | OIDC provider                                 | ✔️          |
| `rekor`                               | transparency log for `cosign sign`            | ✔️          |
| `redis`                               | Rekor's search index                          |             |
| `trillian-server` + `trillian-signer` | Rekor's backing store (a Merkle tree)         |             |
| `mysql`                               | Trillian's backing store                      |             |

All the containers in the pod start at the same time.
Those that depend on other services simply crash-loop until their dependencies are available.

Note that, because `podman kube play` restarts containers with 0 delay and no backoff,
the crash-looping can create a lot of noise in logs.
When dumping the Sigstore pod logs (when a host-facing service fails to initialize in time),
the test framework trims the logs of noisy services down to the last two restarts.

#### Signing keys

Some of the components need signing keys and/or certificates to function.
The Pod YAML does not include those, instead it references external ConfigMaps.
The test framework generates the ConfigMaps before starting the pod
and supplies them using `podman kube play --configmap`.

Generated per run:

* Fulcio CA cert and key
* CTFE signing key
* Rekor signing key
* Public keys for Rekor and CTFE, served from the TUF root along with the Fulcio CA cert

#### OIDC token

The testing Sigstore stack configures Dex with a [`mockCallback` connector][dex-mock-callback]
that auto-approves every request and always returns the fake identity `kilgore@kilgore.trout`.

Tests can get an OIDC token at any time using the `GetOIDCToken` helper method
and pass it to `konflux-build-cli` as appropriate.

#### OIDC url

The OIDC issuer URL (`http://localhost:8888/auth`) must exactly match between the Dex config,
the Fulcio config and the `--oidc-issuer` cosign flag used when performing the signing.

This means the URL must be valid and point to the same host in two different network namespaces:
the network namespace inside the Podman pod and the host network (`cosign sign` runs on the host network).
This is easy to achieve with a Podman pod (services talk to each other on `localhost` inside the pod)
but would be harder with some of the other alternatives.

## Rejected alternatives

### Sigstore's public good instance

Sigstore maintains a publically available deployment (the root is <https://tuf-repo-cdn.sigstore.dev/>).
This is not a good choice for testing. The tests would need to authenticate using a supported provider
(GitHub, Google, ...), which would make them harder to run locally and require secret management in CI.

### Kind cluster

The tests could deploy Sigstore using the same Helm chart that konflux-ci/konflux-ci uses:
[integrations/sigstore/install.sh][konflux-ci-sigstore-install].
This saves the need to define a custom Pod, but also:

* Introduces the dependency on `kind`.
* Takes longer and requires more resources to deploy.
* Requires a workaround to expose the OIDC issuer under the same URL inside and outside the cluster
  (see [OIDC url](#oidc-url)).
* Requires a workaround to enable getting OIDC tokens from outside the cluster
  (in the default configuration, only pods inside the cluster can get the token).
  * Alternatively, would require the keyless signing tests to run as pods inside the cluster
    too, which would further complicate the testing framework.

### Docker Compose

Defining the Sigstore stack as a Docker Compose yaml would have similar benefits as a Podman pod,
but wouldn't work out of the box for Podman users. Podman's `compose` subcommand supports two
different providers:

* The actual `docker-compose` (which needs the systemd `podman.socket` unit enabled).
* The `podman-compose` script (which tends to be more buggy).

The fact that Podman supports two providers makes this a headache for future maintenance,
since each provider is likely to have different bugs.
And the compose would have to make use of more advanced networking features to expose the OIDC issuer
under the same URL inside and outside the compose (see [OIDC url](#oidc-url)).
Which is where the provider differences would be more likely to introduce problems.

Considering that Docker support is already second-class
(CI runs with podman, tests often break for Docker with nobody noticing),
fragmenting the test framework in an attempt to keep Docker support is not worth it.

### Bare containers

The tests could deploy the Sigstore stack as individual containers, not encapsulated in a Podman pod.
This would make them work with Docker, at the cost of:

* More complex setup and teardown code.
  What is currently a declarative Pod yaml would have to be custom framework code.
* More host ports required to run the stack, since all the containers would use the host network
  (or even more complex setup code to create the isolated network and expose the required ports).
* Less convenient debugging (`podman pod` commands make it easy to work with the pod as a whole).

[buildah-keyless-code]: https://github.com/konflux-ci/container-build-catalog/blob/cab160f4afed001a6ad32f4b0e1ee3067d1b8547/task/buildah/buildah.yaml#L816
[sigstore/scaffolding]: https://github.com/sigstore/scaffolding/
[dex-mock-callback]: https://github.com/dexidp/dex/blob/98e0af563213b163ccaed6bd1b8314e2fa22c641/connector/mock/connectortest.go#L15
[konflux-ci-sigstore-install]: https://github.com/konflux-ci/konflux-ci/blob/6d6e8be9aa6b4ef783469111aca14062e7fb8cda/integrations/sigstore/install.sh
