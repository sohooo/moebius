FROM golang:1.25-alpine AS build

ARG MOBIUS_VERSION=latest
ARG MOBIUS_COMMIT=unknown
ARG MOBIUS_BUILD_DATE=unknown
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} GOBIN=/go/bin go install -trimpath -ldflags="-s -w -X github.com/sohooo/moebius/internal/buildinfo.Version=${MOBIUS_VERSION} -X github.com/sohooo/moebius/internal/buildinfo.Commit=${MOBIUS_COMMIT} -X github.com/sohooo/moebius/internal/buildinfo.Date=${MOBIUS_BUILD_DATE}" github.com/sohooo/moebius/cmd/mobius@${MOBIUS_VERSION}

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=build /go/bin/mobius /usr/local/bin/mobius
RUN ln -s /usr/local/bin/mobius /usr/local/bin/møbius

ENTRYPOINT ["/usr/local/bin/møbius"]
CMD ["diff"]
