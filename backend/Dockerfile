FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/contribution-retention ./cmd/contribution-retention

FROM alpine:3.24
ARG REVISION=development
LABEL org.opencontainers.image.source="https://git.subcult.tv/subculture-collective/invisible-museum" \
      org.opencontainers.image.revision=$REVISION
RUN apk add --no-cache ca-certificates && adduser -D -H appuser && mkdir -p /data/media && chown appuser /data/media
USER appuser
COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate
COPY --from=build /out/contribution-retention /contribution-retention
COPY migrations /migrations
ENV MEDIA_FS_DIR=/data/media
EXPOSE 8080
ENTRYPOINT ["/api"]
