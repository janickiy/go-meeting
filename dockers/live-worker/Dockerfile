FROM golang:1.26.6-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/live-worker ./cmd/live-worker
FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates ffmpeg && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /bin/live-worker /app/live-worker
COPY database/migrations /app/database/migrations
USER app
EXPOSE 8093
ENTRYPOINT ["/app/live-worker"]
