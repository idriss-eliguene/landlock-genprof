FROM alpine:3.18 AS build

RUN apk add --no-cache build-base
WORKDIR /src
COPY test/e2e/seccomp-probe.c .
RUN cc -Os -static -s -o /seccomp-probe seccomp-probe.c

FROM curlimages/curl:8.3.0

COPY --from=build /seccomp-probe /usr/local/bin/seccomp-probe
