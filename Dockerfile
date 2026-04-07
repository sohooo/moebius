FROM golang:1.25-alpine AS build

ARG MOBIUS_VERSION=latest
ARG MOBIUS_COMMIT=unknown
ARG MOBIUS_BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -trimpath -ldflags="-s -w -X github.com/sohooo/moebius/internal/buildinfo.Version=${MOBIUS_VERSION} -X github.com/sohooo/moebius/internal/buildinfo.Commit=${MOBIUS_COMMIT} -X github.com/sohooo/moebius/internal/buildinfo.Date=${MOBIUS_BUILD_DATE}" github.com/sohooo/moebius/cmd/mobius@${MOBIUS_VERSION}

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=build /go/bin/mobius /usr/local/bin/møbius

ENTRYPOINT ["/usr/local/bin/møbius"]
CMD ["diff"]
