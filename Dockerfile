FROM node:24-bookworm-slim AS frontend
WORKDIR /web
COPY frontend/package*.json ./
RUN --mount=type=tmpfs,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN --mount=type=tmpfs,target=/tmp GOCACHE=/tmp/go-cache CGO_ENABLED=0 go build -trimpath -o /server ./cmd/server

FROM alpine:3.23
WORKDIR /app
COPY --from=backend /server /app/server
COPY --from=frontend /web/out /app/public
ENV STATIC_DIR=/app/public
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/server"]
