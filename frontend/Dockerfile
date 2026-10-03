FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html tsconfig.json vite.config.ts ./
COPY src ./src
COPY public ./public
RUN npm run build

FROM nginx:1.27-alpine AS runtime
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY 15-csp-origins.envsh /docker-entrypoint.d/15-csp-origins.envsh
RUN chmod +x /docker-entrypoint.d/15-csp-origins.envsh
COPY --from=build /app/dist /usr/share/nginx/html
ENV API_UPSTREAM=api:8085
ENV APP_ENV=production
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
