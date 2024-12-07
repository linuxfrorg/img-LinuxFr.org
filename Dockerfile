# syntax=docker/dockerfile:1

# Build
FROM docker.io/golang:1.23.4-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -trimpath -o /img-LinuxFr.org

RUN go install golang.org/x/vuln/cmd/govulncheck@latest \
  && govulncheck -show verbose ./... \
  && govulncheck -show verbose --mode=binary /img-LinuxFr.org

RUN apk add --no-cache tzdata=2024b-r1

# Deploy
FROM docker.io/alpine:3.20.3
USER 1000

LABEL "org.opencontainers.image.source"="https://github.com/linuxfrorg/img-LinuxFr.org"
LABEL "org.opencontainers.image.description"="Reverse-proxy cache for external domains on LinuxFr.org"
LABEL "org.opencontainers.image.licenses"="AGPL-3.0-only"

WORKDIR /

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /img-LinuxFr.org /img-LinuxFr.org


EXPOSE 8000

# variable not interpreted with JSON format
# hadolint ignore=DL3025
CMD /img-LinuxFr.org -r ${REDIS:-redis:6379/0} -d ${CACHE:-cache} -l ${LOGFILE:--} -a ${ADDR:-127.0.0.1:8000} -e ${AVATAR:-//nginx/default-avatar.svg}
