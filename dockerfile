FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY ./internal ./internal

RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -o main ./internal/main.go

FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates
COPY --from=builder /app .

ENTRYPOINT ["/app/main"]