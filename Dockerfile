FROM golang:1.24-alpine AS base

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY *.go .
RUN go build -o notifier

FROM alpine:3.21
COPY --from=0 /build/notifier /notifier
CMD ["/notifier"]
