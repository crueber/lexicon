# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /build/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.25-alpine AS backend-builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Copy built frontend into the embed location.
COPY --from=frontend-builder /build/web/dist ./internal/server/dist

# Build a fully static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -o /lexicon ./cmd/lexicon

# Stage 3: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata su-exec

# Create a non-root user.
RUN addgroup -S lexicon && adduser -S lexicon -G lexicon

COPY --from=backend-builder /lexicon /usr/local/bin/lexicon
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 6060

VOLUME ["/app/data", "/books", "/bookdrop"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["lexicon"]
