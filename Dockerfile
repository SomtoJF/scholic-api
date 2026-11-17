# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build deps
RUN sed -i 's#dl-cdn.alpinelinux.org#dl-2.alpinelinux.org#g' /etc/apk/repositories && \
    apk add --no-cache git ca-certificates

# Cache deps before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN ls -la /app && ls -la /app/cmd || echo "NO CMD DIR"

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o scholic_api ./cmd/api

# ── Stage 2: Run ──────────────────────────────────────────────────────────────
FROM alpine:3.22

WORKDIR /app

# ca-certificates needed for outbound HTTPS (Google OAuth, R2, etc.).
# poppler-utils provides pdfinfo/pdftotext for bounded document metadata extraction.
RUN sed -i 's#dl-cdn.alpinelinux.org#dl-2.alpinelinux.org#g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata poppler-utils

COPY --from=builder /app/scholic_api .

EXPOSE 8080

CMD ["./scholic_api"]
