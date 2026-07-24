# =========================================================================
# STAGE 1: Install packages into rootfs using --installroot pattern
# =========================================================================
FROM registry.access.redhat.com/ubi9/ubi:latest AS rpm-builder

RUN mkdir -p /mnt/rootfs && \
    dnf install --installroot /mnt/rootfs \
        bash \
        coreutils-single \
        tar \
        gzip \
        curl \
        skopeo \
        sed \
        gawk \
        grep \
        findutils \
        ca-certificates \
        --releasever 9 --setopt=install_weak_deps=0 --nodocs -y && \
    dnf --installroot /mnt/rootfs clean all && \
    rm -rf /mnt/rootfs/var/cache/* /mnt/rootfs/var/log/* /mnt/rootfs/tmp/*

# Add non-root user
RUN echo "runner:x:1001:0:runner:/:/sbin/nologin" >> /mnt/rootfs/etc/passwd

# =========================================================================
# STAGE 2: Runtime (Red Hat UBI Micro)
# =========================================================================
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

COPY --from=rpm-builder /mnt/rootfs /

COPY hack/install_plugins.sh /usr/local/bin/install_plugins.sh
RUN chmod +x /usr/local/bin/install_plugins.sh

USER 1001

ENTRYPOINT ["/bin/bash", "/usr/local/bin/install_plugins.sh"]
