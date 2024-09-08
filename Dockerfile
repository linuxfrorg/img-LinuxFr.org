# syntax=docker/dockerfile:1

# Build
FROM docker.io/golang:1.23.1-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -trimpath -o /img-LinuxFr.org

RUN go install golang.org/x/vuln/cmd/govulncheck@latest
RUN govulncheck -show verbose ./...
RUN govulncheck -show verbose --mode=binary /img-LinuxFr.org

RUN apk add tzdata

# Deploy
FROM docker.io/alpine
USER 1000

WORKDIR /

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /img-LinuxFr.org /img-LinuxFr.org


EXPOSE 8000

CMD /img-LinuxFr.org -r ${REDIS:-redis:6379/0} -d ${CACHE:-cache} -l ${LOGFILE:--} -a ${ADDR:-127.0.0.1:8000} -e ${AVATAR:-//nginx/default-avatar.svg}
