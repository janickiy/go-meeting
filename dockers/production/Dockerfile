FROM nginx:1.30.5-alpine AS runtime
# Nginx остаётся в поддерживаемой ветке; обновляются уязвимые expat/pcre2.
RUN apk upgrade --no-cache
