# syntax=docker/dockerfile:1
# Build context is the repo root (so pnpm workspace files are visible).
# Build: docker build -f web/Dockerfile --secret id=heroui_token,env=HEROUI_AUTH_TOKEN -t fnmdp-web .
FROM node:24-alpine AS build
RUN corepack enable
WORKDIR /app
# Workspace manifests first for better layer caching.
COPY pnpm-workspace.yaml pnpm-lock.yaml ./
COPY web/package.json ./web/
# @heroui-pro postinstall needs HEROUI_AUTH_TOKEN to fetch Pro artifacts. Mounted
# as a BuildKit secret so the token never lands in an image layer; absent (e.g.
# local builds without the secret) install still runs, only Pro fetch is skipped.
RUN --mount=type=secret,id=heroui_token \
    HEROUI_AUTH_TOKEN="$(cat /run/secrets/heroui_token 2>/dev/null || true)" \
    pnpm install --frozen-lockfile
COPY web/ ./web/
RUN pnpm --dir web build

FROM nginx:1.27-alpine
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/web/dist /usr/share/nginx/html
EXPOSE 8897
