# subscription-manager support

Exploration of the Konflux support for subscription-manager
and design for how `konflux-build-cli image build` should implement it.

## Context

Subscription-manager makes it possible to `dnf install` subscription-gated RPMs.

### How it works

* `/etc/pki/entitlement/`
  * contains the client certs that enable the installation to succeed
  * created by `subscription-manager register`
* `/etc/pki/consumer`
  * the "identity" of a registration, enables subscription-manager to modify/revoke the registration
  * created by `subscription-manager register`
* `/etc/rhsm/ca/`
  * contains the CA cert(s) of the servers that serve the subscription-gated RPMs
  * comes with the `subscription-manager-certificates` RPM
* `/etc/yum.repos.d/redhat.repo`
  * the repo file that tells dnf where to look for subscription-gated content
    and what client/CA certs to use
  * synthesized from `/etc/pki/entitlement` automagically by the subscription-manager dnf plugin

> [!NOTE]
> You don't need to have subscription-manager (or the dnf plugin) installed for this to work.
> The handling is built into librhsm (used in libdnf), so dnf and microdnf just work out of the box.
> All that's needed is the entitlement certs and the CA cert.

### How it works in containers (host integration)

* The **host machine** may have `/usr/share/containers/mounts.conf`
  (comes from the `containers-common` RPM) with the following content:

  ```text
  /usr/share/rhel/secrets:/run/secrets
  ```

* `/usr/share/rhel/secrets` (also from `containers-common`) is:

  ```text
  /usr/share/rhel/secrets
  ├── etc-pki-entitlement -> ../../../../etc/pki/entitlement
  ├── redhat.repo -> ../../../../etc/yum.repos.d/redhat.repo
  └── rhsm -> ../../../../etc/rhsm
  ```

  Meaning that the container engine will mount:
  * /etc/pki/entitlement -> /run/secrets/etc-pki-entitlement
  * /etc/rhsm -> /run/secrets/rhsm

  And also the redhat.repo file, but that is less important.
  If `subscription-manager` is installed, it manages the repo file itself.
  If not, modern dnf synthesizes it from the entitlement certs anyway.

* The **container image** may have (UBI usually does):

  ```text
  /etc/pki/entitlement-host -> /run/secrets/etc-pki-entitlement
  /etc/rhsm-host -> /run/secrets/rhsm
  ```

* If the `*-host` paths exist and are directories (or symlinks to directories):
  * subscription-manager operates in container mode, meaning:
    * the subscription-manager CLI always exits with an error
    * the dnf plugin uses the `*-host` paths instead of the regular paths
  * librhsm uses the `*-host` paths instead of the regular paths
    (relevant if subscription-manager isn't installed)

### How it works in Konflux

The Konflux buildah task provides two mechanisms for access to subscription-gated RPMs.

#### Entitlement cert secret

The user runs `subscription-manager register` on their machine,
takes the `/etc/pki/entitlement` certs and stores them in a secret.
The buildah task takes the certs and mounts them at `/etc/pki/entitlement` during the build.

> [!NOTE]
> The mount path is wrong, dnf prefers the (empty) `/etc/pki/entitlement-host` dir.
> It works by coincidence, thanks to a change introduced in [build-definitions@065b74b]
> (the one that disables the host integration, not the one that matches the commit message).

This approach is discouraged, because the entitlement server sometimes revokes certificates
as part of regular operations. But the approach is still supported.

#### Activation key secret

The user obtains an activation key and organization ID and stores them in a secret.
The buildah task mounts them at `/activation-key/activationkey` and `/activation-key/org`.
In the containerfile, the user runs:

```bash
subscription-manager register \
  --org="$(cat /activation-key/org)" \
  --activationkey="$(cat /activation-key/activationkey)"
```

The buildah task mounts empty directories over `/etc/pki/entitlement` and `/etc/pki/consumer`.
This prevents the secrets created as a result of `subscription-manager register` from staying
in the built image (they instead go to the mounted directories).

This should not have worked because subscription-manager refuses to operate in a container.
Users likely soon faced problems, resulting in one of the changes in [build-definitions@065b74b]
(the one that disables host integration, making subscription-manager think it's not in a container).

Optionally, the buildah task may pre-register for the user. The task makes a regex-based guess and,
if it doesn't look like the containerfile runs `subscription-manager register`, then the task
runs the registration itself and mounts the outputs at `/etc/pki/{entitlement,consumer}`.
The task still mounts the secrets at `/activation-key`, so the build has the ability to re-register.

For the pre-registration path, the task also mounts `/etc/rhsm/ca/redhat-uep.pem` into the build.
The reasoning for why this happens (or why it doesn't happen for the other paths) was never explained,
but the logic is likely this: If the containerfile doesn't run `subscription-manager register`,
then there's a chance the `subscription-manager-certificates` RPM isn't installed in the base image,
so mount the CA cert in case the image doesn't have it. The same logic would apply to the entitlement
cert secret approach, where the mount doesn't happen. That's most likely a bug.

## konflux-build-cli implementation

### Disabling host integration

Proper functioning of the subscription-manager support requires host integration to be disabled.
The buildah task does it by deleting `/usr/share/rhel/secrets`, but this is a destructive operation
that requires root permissions. Unacceptable for a local CLI tool.

Konflux-build-cli will disable host integration by mounting a tmpfs over `/usr/share/rhel/secrets`
inside an user+mount namespace (`unshare --map-root-user --mount` or equivalent).
This solves both problems:

* Inside the user namespace, the subprocess will run as root, allowing it to create mounts
  even if `konflux-build-cli` is running as non-root
* The mount namespace ensures the effects are local to the subprocess and don't affect the host

Konflux-build-cli already wraps the `buildah build` call in multiple levels of wrappers,
e.g. `buildah unshare -- konflux-build-cli internal in-user-namespace -- buildah build ...`.
The `internal in-user-namespace` command will get a new `--rhsm-disable-host-integration` flag
with the effect of mounting a tmpfs over the `/usr/share/rhel/secrets` directory if it exists.

Same as the buildah task, the CLI will *always* disable host integration,
even if the user doesn't request any subscription-manager features.
This is to avoid unexpected differences in behavior between registered and unregistered hosts.
We may expose a flag to toggle the disabling in the future, if necessary.

### Pre-registration

The regex-based guess in the current buildah task is incredibly fragile.
The CLI will not implement it. Instead, the CLI will take `--rhsm-activation-preregister=true|false`.
To keep backwards compatibility, the regex-based guess will stay on the Tekton task level.

Also note that pre-registration will work only when running `konflux-build-cli` as root.
There was an attempt to make `subscription-manager register` work inside a user+mount namespace,
but subscription-manager needs read-write access to many root-owned files, which makes this
approach not viable.

### RHSM CA cert

The buildah task's handling of the CA cert doesn't make a lot of sense. To improve coherence,
the CLI will take `--rhsm-mount-ca-cert=always|auto|never`.

* `always` always mounts the cert, fails if it doesn't exist on the host
* `auto` mounts the cert for the activation key self-registration path (like the current buildah task)
  and the entitlement cert path (unlike the current buildah task, which forgets to do this).
  If the cert doesn't exist on the host, logs a warning and proceeds.
* `never` never mounts the cert

Also, the CLI will mount the whole `/etc/rhsm/ca` directory instead of a specific file.
This addresses [build-definitions#1621].

## Sources

libdnf

* [`dnf_content_setup_enrollments`][dnf_content_setup_enrollments]: synthesizes redhat.repo
  from entitlement certs if subscription-manager isn't installed

librhsm

* [`rhsm_context_constructed`][rhsm_context_constructed]: sets up RHSM configuration,
  prefers `*-host` paths over regular paths

subscription-manager

* [`in_container`][subman_in_container]: checks if subscription-manager is running in a container
  (the comment is misleading, the function returns true even if /etc/pki/entitlement-host is empty)
* [`main`][subman_main]: exits immediately if running in a container

buildah task

* [L787-L832][buildah-task-subman-lines]: subscription-manager support code
* [L890][buildah-task-disable-host-integration]: disables host integration

Konflux docs

* [entitlement-subscription]: describes the entitlement secret approach
* [activation-keys-subscription]: describes the activation key secret approach

<!-- links table -->
[dnf_content_setup_enrollments]: https://github.com/rpm-software-management/libdnf/blob/c885cc5a6c34a8941cc8a447b09145bb23292a53/libdnf/dnf-context.cpp#L2190
[rhsm_context_constructed]: https://github.com/rpm-software-management/librhsm/blob/29b68abbf0bf122aeda239c69578e0503dbbe957/rhsm/rhsm-context.c#L553
[subman_in_container]: https://github.com/candlepin/subscription-manager/blob/f4e41e55039a59b1deabb39aa549b8e77f475df5/src/rhsm/config.py#L108
[subman_main]: https://github.com/candlepin/subscription-manager/blob/f4e41e55039a59b1deabb39aa549b8e77f475df5/src/subscription_manager/cli_command/cli.py#L241
[build-definitions@065b74b]: https://github.com/konflux-ci/build-definitions/commit/065b74b8cc9bc99eead722ce43b6b1f7461c6e5c
[build-definitions#1621]: https://github.com/konflux-ci/build-definitions/issues/1621
[buildah-task-subman-lines]: https://github.com/konflux-ci/build-definitions/blob/0628a2a4c9dc439584237bfd5a47fa3626c237ad/task/buildah/0.9/buildah.yaml#L787-L832
[buildah-task-disable-host-integration]: https://github.com/konflux-ci/build-definitions/blob/0628a2a4c9dc439584237bfd5a47fa3626c237ad/task/buildah/0.9/buildah.yaml#L890
[entitlement-subscription]: https://konflux-ci.dev/docs/building/entitlement-subscription/
[activation-keys-subscription]: https://konflux-ci.dev/docs/building/activation-keys-subscription/
