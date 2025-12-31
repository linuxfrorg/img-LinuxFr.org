# syntax=docker/dockerfile:1

# Build
FROM docker.io/golang:1.25.5-alpine3.23 AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go vet \
  && go build -trimpath -o img-LinuxFr.org

RUN go install golang.org/x/vuln/cmd/govulncheck@latest \
  && govulncheck -show verbose ./... \
  && govulncheck -show verbose --mode=binary img-LinuxFr.org

RUN apk add --no-cache tzdata=2025c-r0

# Deploy
FROM docker.io/alpine:3.23.2
ARG UID=1000
ARG GID=1000
RUN addgroup -g "${GID}" app \
  && adduser -D -g '' -h /app -s /bin/sh -u "${UID}" -G app app \
  && install -d -o app -g app -m 0755 /app/cache
USER app

LABEL "org.opencontainers.image.source"="https://github.com/linuxfrorg/img-LinuxFr.org"
LABEL "org.opencontainers.image.description"="Reverse-proxy cache for external domains on LinuxFr.org"
LABEL "org.opencontainers.image.licenses"="AGPL-3.0-only"

WORKDIR /app

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build --chown=app:app /app/img-LinuxFr.org ./


EXPOSE 8000

ENTRYPOINT ["/app/img-LinuxFr.org"]
CMD ["--help"]
