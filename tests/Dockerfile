# syntax=docker/dockerfile:1

FROM ghcr.io/orange-opensource/hurl:6.0.0

LABEL "org.opencontainers.image.source"="https://github.com/linuxfrorg/img-LinuxFr.org"
LABEL "org.opencontainers.image.description"="Tests suite for reverse-proxy cache for external domains on LinuxFr.org"
LABEL "org.opencontainers.image.licenses"="AGPL-3.0-only"

WORKDIR /app

COPY tests_misc.hurl ./
COPY tests_img.hurl ./
COPY tests_avatars.hurl ./
COPY tests_img_after_fetch_before_cache_expiration.hurl ./
COPY tests_img_after_fetch_and_cache_expiration.hurl ./
COPY img-tests.sh ./

RUN apk add --no-cache \
      bash=5.2.15-r0 \
      coreutils=9.1-r0 \
      docker=20.10.24-r2 \
      xxd=9.0.0999-r0 \
      shellcheck=0.8.0-r1 \
      bind-tools=9.18.31-r0 \
    && shellcheck /app/img-tests.sh \
    && mkdir -p cache-img

ENTRYPOINT ["/app/img-tests.sh"]
