#!/bin/sh
# Запускает S3-хранилище только с заданными ключами, не выводя их в журнал.
# Порт, каталог и лимит томов берутся из окружения.
set -eu
: "${AWS_ACCESS_KEY_ID:?S3 access key required}"
: "${AWS_SECRET_ACCESS_KEY:?S3 secret key required}"
: "${S3_BUCKET:?S3 bucket required}"
exec weed mini \
  -dir=/data -ip=127.0.0.1 -ip.bind=0.0.0.0 -s3.port=9000 \
  -s3.iam=false -s3.iam.readOnly=true -s3.autoCreateBucket=false \
  -s3.allowDeleteBucketNotEmpty=false -s3.port.iceberg=0 -s3.port.lance=0 \
  -admin.ui=false -webdav=false -disableHttp=true -master.telemetry=false \
  -filer.disableDirListing=true -filer.exposeDirectoryData=false \
  -master.volumeSizeLimitMB=128 -volume.max="${S3_MAX_VOLUMES:-16}" \
  -s3.readerCacheSizeMB=32 -volume.concurrentUploadLimitMB=64 \
  -volume.concurrentDownloadLimitMB=64
