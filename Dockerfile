# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS build

ARG MOBIUS_VERSION=dev
ARG MOBIUS_COMMIT=unknown
ARG MOBIUS_BUILD_DATE=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64

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

FROM alpine:3.22

RUN apk add --no-cache ca-certificates git

COPY --from=build /out/mobius /usr/local/bin/mobius
RUN ln -s /usr/local/bin/mobius /usr/local/bin/møbius

ENTRYPOINT ["/usr/local/bin/møbius"]
CMD ["diff"]
