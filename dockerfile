FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./internal ./

RUN CGO_ENABLED=0 GOOS=linux go build -o main ./main.go

FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates
COPY --from=builder /app/main .

ENTRYPOINT ["/app/main"]