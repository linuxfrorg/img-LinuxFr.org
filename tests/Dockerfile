# syntax=docker/dockerfile:1

FROM ghcr.io/orange-opensource/hurl:4.3.0

WORKDIR /tmp

COPY tests_misc.hurl ./
COPY tests_img.hurl ./
COPY tests_avatars.hurl ./
COPY tests_img_after_fetch.hurl ./
COPY tests_img_after_fetch.hurl ./
COPY img-tests.sh ./

RUN apk add --no-cache bash xxd docker

RUN mkdir -p nginx-data cache-img

ENTRYPOINT /tmp/img-tests.sh
