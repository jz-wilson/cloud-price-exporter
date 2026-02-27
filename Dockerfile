FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go test ./...

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o cloud-price-exporter .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 exporter

COPY --from=builder /app/cloud-price-exporter /bin/

USER exporter

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/ || exit 1

ENTRYPOINT ["/bin/cloud-price-exporter"]
