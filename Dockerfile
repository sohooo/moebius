FROM golang:1.25-alpine AS build

ARG MOBIUS_VERSION=latest

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -trimpath -ldflags="-s -w" github.com/sohooo/moebius/cmd/mobius@${MOBIUS_VERSION}

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=build /go/bin/mobius /usr/local/bin/møbius

ENTRYPOINT ["/usr/local/bin/møbius"]
CMD ["diff"]
