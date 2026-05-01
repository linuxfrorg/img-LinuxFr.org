# syntax=docker/dockerfile:1

FROM ghcr.io/orange-opensource/hurl:8.0.1

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
      bash=5.3.3-r1 \
      coreutils=9.8-r1 \
      docker=29.1.3-r4 \
      xxd=9.2.0389-r0 \
      shellcheck=0.11.0-r0 \
      bind-tools=9.20.22-r0 \
    && shellcheck /app/img-tests.sh \
    && mkdir -p cache-img

ENTRYPOINT ["/app/img-tests.sh"]
