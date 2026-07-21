FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY ./internal ./internal
COPY config.yml ./

RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -o main ./internal/main.go

FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates
COPY --from=builder /app/main /app/main
COPY --from=builder /app/config.yml /app/config.yml

ENTRYPOINT ["/app/main"]
