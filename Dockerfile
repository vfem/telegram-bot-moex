# Build stage
FROM golang:1.27-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bot ./cmd/bot

# Minimal deployment image (<20MB)
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bot /app/bot

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/bot"]
