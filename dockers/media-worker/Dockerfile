FROM golang:1.24-alpine3.22 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/media-worker ./cmd/media-worker

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /bin/media-worker /app/media-worker
USER app
EXPOSE 8091
ENTRYPOINT ["/app/media-worker"]
