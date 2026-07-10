FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/api ./cmd/api && \
    CGO_ENABLED=0 go build -o /build/worker ./cmd/worker

FROM node:22-alpine AS frontend-builder
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/api .
COPY --from=builder /build/worker .
COPY --from=frontend-builder /build/dist ./frontend/dist
EXPOSE 8080
CMD ["./api"]
