FROM golang:1.26.6-alpine3.23 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG BUILD_VERSION=dev
RUN printf '%s' "$BUILD_VERSION" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$' \
    && CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-X github.com/janickiy/go-recorder/internal/usecase/platform.ReleaseVersion=$BUILD_VERSION" \
      -o /bin/go-recorder ./cmd/api

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S app -G app
WORKDIR /app

COPY --from=build /bin/go-recorder /app/go-recorder
COPY database/migrations /app/database/migrations

USER app
EXPOSE 8085

ENTRYPOINT ["/app/go-recorder"]

FROM golang:1.26.6-alpine3.23 AS debug

RUN apk add --no-cache ca-certificates git \
	&& go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -gcflags="all=-N -l" -o /app/go-recorder-debug ./cmd/api

EXPOSE 8085 2345

ENTRYPOINT ["/go/bin/dlv", "--listen=:2345", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/app/go-recorder-debug", "--continue"]
