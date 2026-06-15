FROM golang:1.24-alpine3.22 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/go-recorder ./cmd/api

FROM alpine:3.22 AS runtime

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app

COPY --from=build /bin/go-recorder /app/go-recorder
COPY database/migrations /app/database/migrations

USER app
EXPOSE 8085

ENTRYPOINT ["/app/go-recorder"]

FROM golang:1.24-alpine3.22 AS debug

RUN apk add --no-cache ca-certificates git \
	&& go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -gcflags="all=-N -l" -o /app/go-recorder-debug ./cmd/api

EXPOSE 8085 2345

ENTRYPOINT ["/go/bin/dlv", "--listen=:2345", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/app/go-recorder-debug", "--continue"]
