FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/møbius ./cmd/mobius

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=build /out/møbius /usr/local/bin/møbius

ENTRYPOINT ["/usr/local/bin/møbius"]
CMD ["diff"]
