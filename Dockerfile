# Pinned to the go.mod `toolchain` (bump it, this and the Makefile's RACE_IMAGE together): build.sh doesn't pull, so a
# floating tag would silently reuse whatever golang image is cached on the host.
FROM golang:1.26.11-alpine AS builder
LABEL project="go_boilerplate_vs_api_label"
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$VERSION" -o main ./cmd/api/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates     && addgroup -S -g 10001 app     && adduser -S -D -H -u 10001 -G app app
WORKDIR /app
COPY --from=builder /app/main .
# Run unprivileged: a compromised process can't write the binary or act as root
# in the container. The mounted EC keys must be readable by uid 10001.
USER 10001:10001
EXPOSE 8080
CMD ["./main"]
