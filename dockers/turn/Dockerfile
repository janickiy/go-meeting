FROM alpine:3.24.2 AS build

# Закреплённый upstream-релиз; архив проверяется до исполнения configure.
ARG COTURN_VERSION=4.18.0
ARG COTURN_SOURCE_SHA256=28d55294ac596fbd129b293a85e7bb1c5dc4bd15b7fb55c500f355149e5f4e28
RUN apk add --no-cache build-base linux-headers pkgconf curl openssl-dev libevent-dev \
    && curl -fsSL "https://github.com/coturn/coturn/archive/refs/tags/${COTURN_VERSION}.tar.gz" -o /tmp/coturn.tar.gz \
    && echo "${COTURN_SOURCE_SHA256}  /tmp/coturn.tar.gz" | sha256sum -c - \
    && mkdir /src && tar -xzf /tmp/coturn.tar.gz -C /src --strip-components=1
WORKDIR /src
# В проекте используются временные HMAC-учётные данные, а не SQL/Redis/Mongo/OAuth.
# TLS и DTLS остаются включёнными; удалены только неиспользуемые интеграции.
RUN TURN_NO_SQLITE=-DTURN_NO_SQLITE TURN_NO_PQ=-DTURN_NO_PQ \
    TURN_NO_MYSQL=-DTURN_NO_MYSQL TURN_NO_MONGO=-DTURN_NO_MONGO \
    TURN_NO_HIREDIS=-DTURN_NO_HIREDIS TURN_NO_OAUTH=-DTURN_NO_OAUTH \
    TURN_NO_SYSTEMD=-DTURN_NO_SYSTEMD TURN_NO_PROMETHEUS=-DTURN_NO_PROMETHEUS \
    CFLAGS="-O2 -fstack-protector-strong -D_FORTIFY_SOURCE=2" \
    ./configure --prefix=/usr \
    && make -j2 \
    && strip bin/turnserver bin/turnutils_uclient bin/turnutils_peer bin/turnutils_stunclient

FROM alpine:3.24.2 AS runtime
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates openssl libevent \
    && addgroup -g 1000 -S turn && adduser -u 1000 -S turn -G turn
COPY --from=build /src/bin/turnserver /src/bin/turnutils_uclient /src/bin/turnutils_peer /src/bin/turnutils_stunclient /usr/bin/
USER 1000:1000
EXPOSE 3478/tcp 3478/udp 5349/tcp
ENTRYPOINT ["turnserver"]
