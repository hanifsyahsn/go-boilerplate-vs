FROM golang:1.26-alpine AS builder
LABEL project="go_boilerplate_vs_api_label"
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$VERSION" -o main ./cmd/api/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
