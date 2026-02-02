FROM quay.io/konflux-ci/task-runner:1.2.0 AS builder

USER root

RUN skopeo copy --remove-signatures \
        docker://registry.access.redhat.com/ubi10/ubi-minimal:10.1 \
        oci-archive:/my-workdir/baseimage.tar


FROM oci-archive:./baseimage.tar

RUN --mount=type=bind,from=builder,src=.,target=/var/tmp \
    rm /my-workdir/baseimage.tar
