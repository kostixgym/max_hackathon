# Web: Caddy with the static files of the mini-app. Build context is the repository root.

# Build frontend
FROM node:22-alpine AS frontend
WORKDIR /src
ARG VITE_DEV_USER_ID=
ARG VITE_DEV_START_PARAM=
ARG VITE_DEV_USER_FIRST_NAME=
ARG VITE_DEV_USER_LAST_NAME=
ARG VITE_DEV_USER_USERNAME=
ENV VITE_DEV_USER_ID=$VITE_DEV_USER_ID
ENV VITE_DEV_START_PARAM=$VITE_DEV_START_PARAM
ENV VITE_DEV_USER_FIRST_NAME=$VITE_DEV_USER_FIRST_NAME
ENV VITE_DEV_USER_LAST_NAME=$VITE_DEV_USER_LAST_NAME
ENV VITE_DEV_USER_USERNAME=$VITE_DEV_USER_USERNAME
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Serve with Caddy
FROM caddy:2-alpine
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY --from=frontend /src/dist /srv/
