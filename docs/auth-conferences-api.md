# Stage 1: Users, Auth, Conference Domain

Первый этап реализует backend. Отдельным последующим запросом добавлен
[интерфейс React + TypeScript + Vite](frontend.md) в `frontend/`, использующий эти контракты.
Видеосвязь и остальные последующие backend-этапы этим не реализуются.
Recorder smoke page не превращается в интерфейс конференций. WebSocket,
WebRTC-конференции, гостевой вход, SFU, ParticipantSession, чат и composite recording
не добавлены. Существующий recorder-worker и media pipeline остаются без изменений.

## Конфигурация и запуск

```env
JWT_SECRET=<результат openssl rand -hex 32>
HTTP_TRUSTED_PROXIES=
RATE_LIMIT_ENABLED=true
RATE_LIMIT_WINDOW=1m
RATE_LIMIT_AUTH_LOGIN_IP_RPM=10
RATE_LIMIT_AUTH_REGISTER_IP_RPM=5
```

`JWT_SECRET` обязателен только для API: минимум 32 байта, без встроенного значения.
Хранить в локальном `.env` или secret store, не в Git. Смена секрета делает ранее
выданные токены невалидными. Использовать HTTPS вне локального окружения.

По умолчанию forwarded IP headers не считаются доверенными. Если API расположен за
reverse proxy, `HTTP_TRUSTED_PROXIES` — список реальных IP/CIDR через запятую;
не задавать `0.0.0.0/0`. Иначе несколько клиентов за proxy будут иметь общий IP-лимит.
Login и register используют существующий Redis sliding-window limiter до обработки
паролей; при превышении возвращается `429`, при ошибке limiter — `500`
с безопасным сообщением `rate limit unavailable` (существующий контракт limiter).

```bash
docker compose up -d --build api
```

API применяет новые idempotent up-миграции при старте, как и существующие миграции.
Shared config не требует JWT от worker и команды `migrate`.

## Пользователи и JWT

Email обрезается по краям и переводится в нижний регистр, валидируется как один
адрес без display name, максимум 254 байта. Уникальность обеспечена PostgreSQL,
включая конкурентные регистрации. Пароль: 8–128 Unicode-символов, без trim.
Цифры, заглавные буквы и спецсимволы не обязательны. Лимит считается по символам,
а не по UTF-8 байтам; существующие пароли сохраняют совместимость при входе.
`displayName` опционален: trim, максимум 100 символов; пустая строка становится NULL.
Имя участника без displayName — `Participant`, а не его email.

В базе хранится только Argon2id PHC hash: отдельная случайная 16-байтовая соль,
64 MiB памяти, 3 прохода, 4 потока, 32-байтовый результат. Сравнение constant-time.
SQL logger для UserRepository отключён, поэтому ошибка INSERT не выводит hash.
API использует отдельные DTO и никогда не отдаёт password/password_hash.
Основание параметров: [официальная реализация Go Argon2](https://github.com/golang/crypto/blob/master/argon2/argon2.go).

JWT: HS256, `sub` = UUID пользователя, обязательные `iat`/`exp`, TTL 3600 секунд,
`iss=go-recorder`, `aud=go-recorder-api`. Middleware проверяет метод подписи,
подпись, срок, время выдачи, issuer/audience и UUID. Реализация использует
[golang-jwt v5](https://pkg.go.dev/github.com/golang-jwt/jwt/v5@v5.3.1).
Заголовок: `Authorization: Bearer <accessToken>`.

Авторизация аккаунта использует серверную сессию PostgreSQL без срока простоя
или абсолютного истечения. Короткий JWT содержит `sid`; middleware проверяет,
что сессия не отозвана. Login устанавливает HttpOnly cookie, refresh выдаёт
новый JWT, logout отзывает сессию вместе с её JWT. Действующий JWT предыдущего
выпуска мигрирует через `/auth/session`. Подробности: [PERSISTENT_AUTH.md](PERSISTENT_AUTH.md).
Unknown email и wrong password возвращают одинаковый
`401` с `invalid email or password`; неизвестный email тоже выполняет hash-проверку.

## HTTP-контракты

Все пути ниже имеют префикс `/api/v1`. Register/login, cookie refresh/logout и
lookup/гостевой вход по приглашению публичные; остальные маршруты требуют JWT.
JSON имеет camelCase. Идентификатор текущего
пользователя берётся только из auth context, а не из JSON. Неизвестные JSON-поля,
включая `ownerId`, `userId`, `role`, запрещены. Максимальный body — 32 KiB.

| Метод и путь | Тело / результат | Доступ |
| --- | --- | --- |
| `POST /auth/register` | `{email,password,displayName?}` → `201 {status,user}` | public |
| `POST /auth/login` | `{email,password}` → `200 {status,accessToken,tokenType,expiresIn,user}` | public |
| `GET /auth/me` | `200 {status,user}` | authenticated |
| `PATCH /auth/me` | `{displayName}` → `200 {status,user}`; имя после trim: 1–100 Unicode code points, без управляющих символов; другие поля запрещены | authenticated, только свой профиль |
| `POST /auth/refresh` | `{}` → `200 {status,accessToken,tokenType,expiresIn,user}` | HttpOnly cookie, проверка Origin |
| `POST /auth/session` | `{}` → LoginResponse и cookie; миграция действующего старого JWT | authenticated, только постоянный аккаунт |
| `POST /auth/logout` | пустое тело или `{}` → `200 {status,message}`; отзыв cookie/SID или старого JWT | public, идемпотентный, проверка Origin |
| `POST /conferences` | `{title}` → `201 {status,item: Conference}` | authenticated |
| `GET /conferences` | `200 {status,items: Conference[]}` | свои membership |
| `GET /conferences/{id}` | `200 {status,item: Conference}` | membership |
| `POST /conferences/{id}/start` | пустое тело или `{}` → Conference | только owner |
| `POST /conferences/{id}/finish` | пустое тело или `{}` → Conference | только owner |
| `POST /conferences/{id}/cancel` | пустое тело или `{}` → Conference | только owner |
| `POST /conferences/{id}/join` | `{inviteCode?}` или пустое тело → Participant | authenticated + membership/код |
| `POST /conferences/{id}/leave` | пустое тело или `{}` → Participant | membership |
| `GET /conferences/{id}/participants` | `200 {status,items: Participant[]}` | membership |
| `GET /conference-invites/{code}` | `200 {status,item:{id,title,status,waitingRoomEnabled,scheduledAt?}}` | public, no-store |
| `POST /conference-invites/{code}/guest` | `{displayName}` → guest session + Participant (`item`) | public + код, 10/min/IP |
| `POST /conference-invites/{code}/join` | пустое тело или `{}` → Participant | authenticated + код |

Lifecycle/join/leave возвращают `200 {status:"success",item:...}`.
Списки принимают `limit` (1–100, default 20) и `offset` (>=0, default 0).
Список конференций включает сохранённые membership, в том числе `left`;
это не список активных подключений.

```json
{
  "status": "success",
  "user": {
    "id": "<uuid>",
    "email": "owner@example.com",
    "displayName": "Owner",
    "createdAt": "<RFC3339>",
    "updatedAt": "<RFC3339>"
  }
}
```

Conference DTO: `id`, `ownerId`, `title`, `inviteCode`, `inviteUrl`, `status`,
`createdAt`, `updatedAt`, nullable `startedAt`/`finishedAt`. Title: trim, 1–200 символов.
`inviteUrl` — относительный backend lookup URL `/api/v1/conference-invites/{code}`,
не страница звонка. Frontend может построить собственную ссылку по этому коду.
Invite code — 24 криптографически случайных байта, base64url без padding (32 символа,
192 бита); отдельный от UUID, UNIQUE в БД, до 5 попыток при коллизии.
Lookup возвращает только id/title/status и не раскрывает owner, email или участников.

Participant DTO: `id`, `conferenceId`, nullable `userId`, `displayName`, `role`,
`status`, nullable `joinedAt`/`leftAt`, `createdAt`, `updatedAt`.

## Права и состояние

Lifecycle: `created → active → finished` или `created → cancelled`.
Start устанавливает `startedAt`; finish/cancel — `finishedAt`.
Только `ownerId` конференции может менять lifecycle, даже если owner ещё не joined
или уже left. Для чужого пользователя всегда `403`; некорректный переход — `409`.
Co-host management API в этом этапе отсутствует.

При создании конференции в одной транзакции создаётся owner membership:
`role=owner`, `status=left`, `joinedAt=null`, `leftAt=null`.
Здесь `left` означает отсутствие выполненного join, а не историю выхода.
Membership не свидетельствует о WebRTC-подключении или фактическом присутствии.

Первый join другого пользователя требует invite code. Join по UUID без кода
разрешён только для уже существующей membership, включая владельца. Клиент не
может выбрать user или назначить owner/co_host: новая роль всегда `participant`.

Join разрешён только в `created`/`active`. Он устанавливает `status=joined`,
`joinedAt=now`, `leftAt=null`. Уже joined → идемпотентный ответ без изменения времени.
Leave устанавливает `status=left`, `leftAt=now`; повторный leave не меняет время,
а leave до первого join сохраняет обе даты NULL. Rejoin после leave обновляет даты
на той же строке с тем же participant ID. Отдельная история подключений не ведётся.
Membership сохраняет доступ к карточке и списку участников после leave.

Finish/cancel атомарно закрывает все joined membership (`left`, `leftAt=now`),
не удаляя их. Join после терминального статуса возвращает `409`; повторный leave
безопасен. Это доменное закрытие membership, не управление медиасоединением.

Типы ролей: `owner`, `co_host`, `participant`, `guest`. Текущие статусы: `joined`,
`left`; модель/constraints предусматривают `waiting`, `rejected`, `kicked` для
будущего этапа, без соответствующих API. Будущая гостевая строка может иметь
`user_id=NULL, role=guest`; гостевой вход сейчас запрещён JWT middleware.

Все join/leave/lifecycle изменения блокируют строку конференции `FOR UPDATE` внутри
PostgreSQL-транзакции. Это работает между соединениями и экземплярами API;
Go mutex не используется. UNIQUE membership дополнительно запрещает дубликаты.

## Ошибки

Формат: `{ "status": "failed", "message": "<safe message>" }`.

| Код | Значение |
| --- | --- |
| 400 | повреждённый JSON, неизвестные поля, несколько JSON-документов, слишком большой body |
| 401 | нет/неверный/просроченный JWT, неверные login credentials |
| 403 | нет membership/приглашения, чужой owner action |
| 404 | конференция или invite code не найдены |
| 409 | duplicate email, запрещённый lifecycle transition, join закрытой конференции |
| 422 | неверный UUID/email/password/title/пагинация/inviteCode в join body |
| 429 | превышен Redis rate limit |
| 500 | `internal server error` либо `rate limit unavailable`, без SQL/hash/password и деталей инфраструктуры |

## Пример приёмочного сценария

Нужен `jq`. Email/пароль ниже предназначены только для локального примера;
повторная регистрация того же email вернёт `409`.

```bash
STAGE1_BASE=http://localhost:8085/api/v1
curl -sS "$STAGE1_BASE/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"owner@example.com","password":"StrongPassword123","displayName":"Owner"}'
STAGE1_TOKEN=$(curl -sS "$STAGE1_BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"owner@example.com","password":"StrongPassword123"}' | jq -r '.accessToken')
curl -sS "$STAGE1_BASE/auth/me" -H "Authorization: Bearer $STAGE1_TOKEN"
STAGE1_CONFERENCE=$(curl -sS "$STAGE1_BASE/conferences" \
  -H "Authorization: Bearer $STAGE1_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"First conference"}')
STAGE1_ID=$(echo "$STAGE1_CONFERENCE" | jq -r '.item.id')
STAGE1_INVITE=$(echo "$STAGE1_CONFERENCE" | jq -r '.item.inviteCode')
curl -sS "$STAGE1_BASE/conferences/$STAGE1_ID/participants" -H "Authorization: Bearer $STAGE1_TOKEN"
curl -sS -X POST "$STAGE1_BASE/conferences/$STAGE1_ID/join" -H "Authorization: Bearer $STAGE1_TOKEN"
curl -sS -X POST "$STAGE1_BASE/conferences/$STAGE1_ID/start" -H "Authorization: Bearer $STAGE1_TOKEN"
curl -sS -X POST "$STAGE1_BASE/conferences/$STAGE1_ID/leave" -H "Authorization: Bearer $STAGE1_TOKEN"
curl -sS -X POST "$STAGE1_BASE/conferences/$STAGE1_ID/join" -H "Authorization: Bearer $STAGE1_TOKEN"
curl -sS -X POST "$STAGE1_BASE/conferences/$STAGE1_ID/finish" -H "Authorization: Bearer $STAGE1_TOKEN"
```

Для второго пользователя получить отдельный JWT. До finish:

```bash
curl -sS "$STAGE1_BASE/conference-invites/$STAGE1_INVITE" -H "Authorization: Bearer $STAGE1_MEMBER_TOKEN"
curl -sS -X POST "$STAGE1_BASE/conference-invites/$STAGE1_INVITE/join" -H "Authorization: Bearer $STAGE1_MEMBER_TOKEN"
# Любой start/finish/cancel с токеном второго пользователя возвращает 403.
```

## Миграции и совместимость

Добавлены `000006_create_users_table`, `000007_create_conferences_table`,
`000008_create_conference_participants_table` (up/down). PK — UUID; timestamps и
updated_at triggers; UNIQUE email/invite/(conference_id,user_id); один owner-role
на конференцию; lifecycle/identity/status checks; индексы owner и участников.
Удаление user ограничено FK `RESTRICT`; удаление conference каскадно удаляет только
её membership. DELETE API не добавлен. Обычная PostgreSQL UNIQUE-семантика разрешает
несколько будущих guest с NULL user_id. Create conference+owner атомарен.

Миграции не изменяют существующие record/record_files/record_segments/record_events,
не удаляют данные и повторно применяются безопасно. Down только новых таблиц
выполняется в обратном порядке 008→007→006 и удаляет данные Stage 1; его не запускать
без backup и явного решения об откате. Down не применялся при реализации.

`record.conference_id` уже UUID, но исторически принимал произвольные внешние UUID.
Обязательный FK на новую conferences сломал бы старые записи/record.start.
Поэтому FK/backfill не добавлен, прежние recorder endpoints и RabbitMQ-команды не
изменены и пока остаются с прежним контрактом без новой JWT-защиты. Перед публичной
эксплуатацией права на recorder надо согласовать отдельно: авторизация нового
conference API не защищает старые recorder routes.

Новая conference ID может передаваться существующему recorder как conferenceId;
finish конференции сейчас не публикует record.stop и не управляет записью.
Будущий безопасный план связи: инвентаризация внешних UUID, явная таблица соответствий,
nullable canonical FK для legacy/unmatched записей, backfill только подтверждённых
соответствий, затем validation/permissions и постепенная смена recorder contract.
Не создавать фиктивных пользователей/конференций и не переписывать UUID автоматически.

## Проверки

```bash
go test ./...
go vet ./...
go test -race ./...
RECORDER_STAGE1_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
  go test -race ./tests/integration -run TestStageOnePostgres -count=3 -v
```

PostgreSQL-тест требует локальный URL и CREATE DATABASE permission: создаёт собственную
базу `go_recorder_stage1_<random uuid>` и удаляет только её после теста. Не трогает
таблицы основной базы. Проверяет весь сценарий, роли, null dates владельца,
idempotent join, rejoin той же membership, SQL rollback/UNIQUE/FK/NULL, collision retry,
параллельные start/start, finish/cancel и join/finish, отсутствие hash в ответах/SQL logs,
и прежний record.conference_id. Без переменной окружения suite пропускается.

Обычные unit tests проверяют валидацию регистрации, JWT (включая expiration и
подмену алгоритма), соль/hash, генерацию invite, auth/rate-limit middleware,
ошибки и lifecycle. Отдельного lint-инструмента в репозитории нет; CI использует
`go test` и сборку binaries, дополнительно проверяется `go vet`.

Следующий этап — отдельно согласованные WebSocket/signaling и ParticipantSession.
Перед его реализацией определить права co_host, присутствие/мультивкладки,
security recorder API и версионирование миграций. В Stage 1 эти задачи не реализованы.

## Гостевой вход по приглашению (2026-10-05)

`/i/{code}` открывает публичный экран проверки устройств. Для пользователя с аккаунтом
отображается имя профиля, для посетителя — редактируемое «Гость». Просмотр ссылки
не создаёт участника. Кнопка «Подключиться» создаёт гостевую сессию или использует
обычный `/join` для аккаунта. Камера и микрофон включаются явными кнопками; выбранные
устройства подключаются к медиа после допуска во встречу.

Гостевой JWT действует один час и ограничен `guestConferenceId`. Разрешены API этой
встречи, `/auth/me`, `/auth/logout`, `/capabilities`, `/webrtc/config`; общие списки,
создание встреч, поиск и административные API закрыты. WebSocket и ICE проверяют
ту же область доступа. Права участника и зал ожидания действуют без изменений.
Гость не получает роль организатора и не может войти через email/password.

Повторный `/guest` с гостевым Bearer той же встречи сохраняет participant ID и
статус допуска, обновляя имя. Отклонение/исключение не сбрасывается. Новое посещение
без прежней сессии создаёт новую гостевую идентичность по публичному приглашению.
Закрытые и запланированные встречи не принимают новых гостей до открытия.
Миграция `000023_guest_sessions.up.sql` добавляет nullable scope и запрещает
назначать гостю `is_admin`; прежние аккаунты и образы совместимы с этой схемой.

### Уточнение входа по ссылке — 2026-10-05

Камера и микрофон запрашиваются автоматически на публичном экране входа;
браузер сохраняет контроль над разрешениями. Участник может выключить устройства
до подключения или войти без них при отказе в доступе. По действующему приглашению
новые и ранее ожидавшие гости/аккаунты сразу получают `joined/admitted`, независимо
от прежнего флага зала ожидания. Явные отклонения/исключения и жизненный цикл встречи
сохраняются. Миграция 24 допускает ранее ожидавших участников открытых встреч.
