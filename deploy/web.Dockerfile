# Web: Caddy with the static files of the mini-app. Build context is the repository root.
#
# Until step 0.4 the mini-app does not exist: a placeholder page is served.
# Step 0.4 replaces the placeholder with a build stage:
#   FROM node:22-alpine AS miniapp
#   WORKDIR /src
#   COPY miniapp/package.json miniapp/package-lock.json ./
#   RUN npm ci
#   COPY miniapp/ ./
#   RUN npm run build
#   ...
#   COPY --from=miniapp /src/dist /srv

FROM caddy:2-alpine
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY deploy/placeholder/ /srv/
