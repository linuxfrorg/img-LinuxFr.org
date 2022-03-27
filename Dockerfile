# syntax=docker/dockerfile:1

# Build
FROM golang:1.21-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /img-LinuxFr.org

RUN go install golang.org/x/vuln/cmd/govulncheck@latest
RUN govulncheck ./...
RUN govulncheck --mode=binary /img-LinuxFr.org

# Deploy
FROM alpine
ARG REDIS
ENV REDIS=${REDIS:-redis:6379/0}
USER 1000

WORKDIR /

COPY --from=build /img-LinuxFr.org /img-LinuxFr.org

EXPOSE 8000


ENTRYPOINT /img-LinuxFr.org -r ${REDIS}
