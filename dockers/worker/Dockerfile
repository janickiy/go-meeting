FROM golang:1.26.6-alpine3.23 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/go-recorder-worker ./cmd/worker

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates ffmpeg su-exec \
	&& addgroup -S app \
	&& adduser -S app -G app

WORKDIR /app

COPY --from=build /bin/go-recorder-worker /app/go-recorder-worker
COPY database/migrations /app/database/migrations
COPY dockers/worker/entrypoint.sh /app/entrypoint.sh

RUN mkdir -p /storage \
	&& chown -R app:app /storage \
	&& chmod +x /app/entrypoint.sh

EXPOSE 8090

ENTRYPOINT ["/app/entrypoint.sh"]

FROM golang:1.26.6-alpine3.23 AS debug

RUN apk add --no-cache ca-certificates ffmpeg git \
	&& go install github.com/go-delve/delve/cmd/dlv@latest \
	&& mkdir -p /storage

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -gcflags="all=-N -l" -o /app/go-recorder-worker-debug ./cmd/worker

EXPOSE 8090 2346

ENTRYPOINT ["/go/bin/dlv", "--listen=:2346", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/app/go-recorder-worker-debug", "--continue"]
