# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26.9-alpine3.23@sha256:6750308561953d451210ed1a8086d36c8c8bdcbf672d38d9d0d424b8a7a0216c AS build

ARG MOBIUS_VERSION=dev
ARG MOBIUS_COMMIT=unknown
ARG MOBIUS_BUILD_DATE=unknown
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags="-s -w -X github.com/sohooo/moebius/internal/buildinfo.Version=${MOBIUS_VERSION} -X github.com/sohooo/moebius/internal/buildinfo.Commit=${MOBIUS_COMMIT} -X github.com/sohooo/moebius/internal/buildinfo.Date=${MOBIUS_BUILD_DATE}" \
    -o /out/mobius ./cmd/mobius

FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache ca-certificates git

COPY --from=build /out/mobius /usr/local/bin/mobius
RUN ln -s /usr/local/bin/mobius /usr/local/bin/møbius

# Keep root as the GitLab runner compatibility default; support --user overrides.
ENV HOME=/tmp/mobius-home \
    XDG_CACHE_HOME=/tmp/mobius-cache

ENTRYPOINT ["/usr/local/bin/møbius"]
CMD ["diff"]
