FROM --platform=$BUILDPLATFORM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html tsconfig.json vite.config.ts build-metadata.ts ./
COPY src ./src
COPY public ./public
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown
ARG CLIENT_TELEMETRY_ENABLED=false
ARG PRODUCT_NAME=Meetrix
ENV VITE_PRODUCT_NAME=$PRODUCT_NAME
ENV VITE_BUILD_VERSION=$BUILD_VERSION VITE_BUILD_COMMIT=$BUILD_COMMIT VITE_BUILD_TIME=$BUILD_TIME VITE_CLIENT_TELEMETRY_ENABLED=$CLIENT_TELEMETRY_ENABLED
RUN npm run build

FROM nginx:1.30.5-alpine AS runtime
# Исправления системных библиотек устанавливаются до фиксации digest готового образа.
RUN apk upgrade --no-cache
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY 15-csp-origins.envsh /docker-entrypoint.d/15-csp-origins.envsh
RUN chmod +x /docker-entrypoint.d/15-csp-origins.envsh
COPY --from=build /app/dist /usr/share/nginx/html
ENV API_UPSTREAM=api:8085
ENV APP_ENV=production
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
