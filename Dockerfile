FROM node:22-alpine AS frontend
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/storyforge ./cmd/server

FROM alpine:3.23
RUN addgroup -S storyforge && adduser -S -G storyforge storyforge
COPY --from=build /out/storyforge /usr/local/bin/storyforge
COPY --from=frontend /frontend/dist /app/web
USER storyforge
ENV PORT=8080
ENV WEB_DIR=/app/web
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=70s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/readyz" || exit 1
ENTRYPOINT ["/usr/local/bin/storyforge"]
