FROM golang:1.26.6-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X github.com/janickiy/go-recorder/internal/buildinfo.Version=$BUILD_VERSION -X github.com/janickiy/go-recorder/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/janickiy/go-recorder/internal/buildinfo.BuildTime=$BUILD_TIME" \
    -o /bin/product-worker ./cmd/product-worker

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates ffmpeg && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /bin/product-worker /app/product-worker
COPY database/migrations /app/database/migrations
USER app
EXPOSE 8092
ENTRYPOINT ["/app/product-worker"]
