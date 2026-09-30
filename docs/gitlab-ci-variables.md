# GitLab CI/CD variables

Deploy запускается job-ом `deploy-dev` из `.gitlab-ci.yml` для ветки
`feature/1-start-project`.

## Что добавить в GitLab

Открой Project -> Settings -> CI/CD -> Variables -> Add variable.

Обязательные variables:

| Key | Type | Visibility | Value откуда взять |
| --- | --- | --- | --- |
| `DEPLOY_HOST` | Variable | Visible | IP или DNS сервера. Сейчас: `192.168.90.201`. |
| `DEPLOY_USER` | Variable | Visible | Linux-пользователь для SSH. Сейчас: `janickiy`. |
| `DEPLOY_PATH` | Variable | Visible | Каталог на сервере для проекта. Например: `/var/www/go-recorder`. |
| `DEPLOY_SSH_PRIVATE_KEY_B64` | Variable | Masked and hidden | Приватный SSH-ключ deploy-пользователя, закодированный в base64. Рекомендуемый способ. |
| `CI_SOURCE_ARCHIVE_TOKEN_B64` | Variable | Masked and hidden | Project access token со scope `read_api`, закодированный в base64. Нужен, если runner зависает на `git fetch`. |

Вместо `DEPLOY_SSH_PRIVATE_KEY_B64` можно добавить:

- `DEPLOY_SSH_PRIVATE_KEY` с типом `File`, если хочешь хранить ключ как файл.
- `DEPLOY_PASSWORD` с типом `Variable` и visibility `Masked and hidden`, если
  deploy будет идти по паролю.

Лучше использовать отдельный SSH-ключ.

Опциональные variables:

| Key | Type | Visibility | Default | Когда нужен |
| --- | --- | --- | --- | --- |
| `DEPLOY_PORT` | Variable | Visible | `22` | Если SSH слушает нестандартный порт. |
| `DEPLOY_ENV_FILE_B64` | Variable | Masked and hidden | нет | Если нужно управлять серверным `.env` из GitLab. |
| `DEPLOY_ENV_FILE` | File | Visible | нет | Альтернатива для `.env`, если не нужен hidden-режим. |

Base64 нужен потому, что `Masked and hidden` variables в GitLab должны быть в
одну строку. Для secret variables отключи `Expand variable reference`. В GitLab
API это поле называется `raw=false`. `Protected` включай только если ветка
`feature/1-start-project` защищена в GitLab. Иначе job не увидит эти variables.

## Как получить SSH-ключ

На своей машине создай отдельный ключ для GitLab deploy:

```bash
ssh-keygen -t ed25519 -C "gitlab-go-recorder-deploy" -f ~/.ssh/go_recorder_gitlab_deploy -N ''
```

Добавь публичный ключ на сервер:

```bash
ssh-copy-id -i ~/.ssh/go_recorder_gitlab_deploy.pub janickiy@192.168.90.201
```

Проверь вход:

```bash
ssh -i ~/.ssh/go_recorder_gitlab_deploy janickiy@192.168.90.201 'whoami && docker --version && docker compose version'
```

В GitLab variable `DEPLOY_SSH_PRIVATE_KEY_B64` вставь приватный ключ в base64:

```bash
base64 < ~/.ssh/go_recorder_gitlab_deploy | tr -d '\n'; echo
```

## Серверный `.env`

Если `DEPLOY_ENV_FILE_B64` и `DEPLOY_ENV_FILE` не заданы, deploy script создаст
`.env` на сервере из `.env.example` только при первом деплое. Для реального
сервера лучше добавить `DEPLOY_ENV_FILE_B64`, собранный из `.env.example`.

Минимально проверь эти значения:

```env
APP_ENV=production
GIN_MODE=release
WEBRTC_NAT_IPS=192.168.90.201
MINIO_PUBLIC_ENDPOINT=192.168.90.201:9000
POSTGRES_PASSWORD=<strong-password>
MINIO_ROOT_PASSWORD=<strong-password>
RABBIT_MQ_HOST=rabbitmq
RABBIT_MQ_PORT=5672
RABBIT_MQ_USER=go_recorder
RABBIT_MQ_PASSWORD=<strong-rabbit-password>
RABBIT_MQ_VHOST=/
RABBIT_MQ_HOST_PORT=5672
RABBIT_MQ_MANAGEMENT_HOST_PORT=15672
```

Подготовь локальный файл, например `.env.production`, и вставь в GitLab
`DEPLOY_ENV_FILE_B64` результат:

```bash
base64 < .env.production | tr -d '\n'
```

RabbitMQ запускается локально в Docker Compose этого проекта, внешняя сеть
`infra_network` больше не нужна. Пользователь, пароль и vhost передаются и
брокеру, и API/worker из одних переменных. Compose принудительно использует
`rabbitmq:5672` и очищает `RABBIT_MQ_DSN` у клиентов, даже если серверный `.env`
сохранил старое подключение. Удали из серверного `.env` устаревшие
`RABBIT_MQ_NETWORK` и `RABBIT_MQ_DSN`.

Host-порты RabbitMQ доступны только через `127.0.0.1`; при конфликте с общим
брокером измени `RABBIT_MQ_HOST_PORT` и `RABBIT_MQ_MANAGEMENT_HOST_PORT`.
Внешняя сеть gateway по-прежнему должна существовать.

При первом переходе заверши активные записи и дождись опустошения старой
очереди, включая неподтвержденные сообщения, до запуска deploy. Сообщения из
общего брокера не переносятся автоматически. Данные нового брокера лежат в
`dockers/rabbitmq/data/` и исключены из `rsync --delete`. После первой
инициализации изменение пароля/vhost в `.env` требует отдельного обновления
пользователя и прав в RabbitMQ; не удаляй каталог данных для смены пароля.

На сервере deploy-пользователь должен иметь право писать в `DEPLOY_PATH` и
запускать `docker compose`.
