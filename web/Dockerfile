# Builds the static admin app and serves it with nginx, proxying /api and /media to the API.
FROM node:22-alpine AS build
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm install -g npm@11 && npm ci --legacy-peer-deps
COPY backend/api/openapi.yaml /backend/api/openapi.yaml
COPY web/ ./
ARG VITE_GOOGLE_CLIENT_ID=
ENV VITE_GOOGLE_CLIENT_ID=$VITE_GOOGLE_CLIENT_ID
RUN npx openapi-typescript /backend/api/openapi.yaml -o src/lib/api/types.ts && npx vite build
ARG REVISION=development
RUN printf '{"revision":"%s"}\n' "$REVISION" > /app/build/release.json

FROM nginx:1.31-alpine
ARG REVISION=development
LABEL org.opencontainers.image.source="https://git.subcult.tv/subculture-collective/invisible-museum" \
      org.opencontainers.image.revision=$REVISION
COPY web/nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/build /usr/share/nginx/html
ENV API_UPSTREAM=http://api:8080
EXPOSE 80
