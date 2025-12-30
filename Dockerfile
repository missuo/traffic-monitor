FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o traffic-monitor .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/traffic-monitor .

EXPOSE 8080

ENTRYPOINT ["/app/traffic-monitor"]
CMD ["-config", "/app/config.yaml"]
