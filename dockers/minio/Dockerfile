FROM golang:1.24-alpine3.22 AS build

# The upstream community images are no longer available from the registries.
# Build the pinned official source release instead of using a third-party image.
ARG MINIO_VERSION=RELEASE.2025-10-15T17-29-55Z
RUN CGO_ENABLED=0 GOBIN=/out go install -p 2 -trimpath -ldflags="-s -w" github.com/minio/minio@${MINIO_VERSION}

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/minio /usr/local/bin/minio

EXPOSE 9000 9001
ENTRYPOINT ["/usr/local/bin/minio"]
