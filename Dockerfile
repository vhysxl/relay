FROM node:20-alpine AS web-builder
WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Copy built frontend assets into internal/web/dist for Go embed.FS
COPY --from=web-builder /app/internal/web/dist ./internal/web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/relay-server .

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=go-builder /app/relay-server .

EXPOSE 8000

CMD ["/app/relay-server"]

