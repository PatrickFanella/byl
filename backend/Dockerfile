FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H appuser && mkdir -p /data/media && chown appuser /data/media
USER appuser
COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate
COPY migrations /migrations
ENV MEDIA_FS_DIR=/data/media
EXPOSE 8080
ENTRYPOINT ["/api"]
