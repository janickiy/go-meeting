# Итог этапа 7

Дата проверки: 2 октября 2026. Реализация по `CODEX_PROMPT_07.md`: уведомления, календарь, расшифровки, итоги ИИ и поиск. Сквозной сценарий подтверждён в изолированном Docker с настоящей записью и явно тестовыми внешними адаптерами. Это не подтверждение качества реального STT/ИИ или готовности конкретного внешнего провайдера к production. Этап 8 не начат.

Настройки, HTTP контракты, API, права и retention описаны в [руководстве этапа 7](../content-integrations.md).

## 1 Реализованные возможности

Добавлены постоянная очередь `background_jobs`, отдельный `product-worker`, email/push preferences и регистрации устройств, UTC reminders, внешний calendar lifecycle, серверный OAuth, асинхронное извлечение аудио и распознавание, timestamped transcript, проверяемые итоги ИИ и PostgreSQL FTS. Frontend получил настройки интеграций, приватный видеоплеер, вкладки текста и итогов, ошибки и разрешённые повторы, поиск и ссылки на конкретную запись без полного редизайна.

До изменений изучены фактические Stage 3–6 реализации и отчёт этапа 6, выполнены baseline unit/race/vet/vulnerability проверки и дан архитектурный отчёт. Шесть существовавших staticcheck замечаний разобраны: механические исправления сохранены отдельно от продуктового поведения; диагностическое ST1005 исключение оставляет совместимый текст ошибки записи.

## 2 Архитектура провайдеров

`EmailProvider`, `PushProvider`, `CalendarProvider`, `TranscriptionProvider`, `AIProvider` отделены от usecase; OAuth также имеет отдельный контракт. Есть noop, явно демонстрационный mock и generic HTTP. Vendor SDK не импортируется в domain. Provider factories проверяют конечные timeout, доверенные operator endpoints и secrets; redirects отключены. Mock запрещён в production.

## 3 Поток email и push

Domain fact сохраняется вместе с заданием, затем формируются личные in-app уведомления и внешние delivery jobs. Fanout ограничен 100 получателями за порцию с постоянным продолжением. Ключи дедупликации стабильны на конференцию, версию, событие и получателя; in-app и audit rows не дублируются при повторном выполнении.

Email имеет экранированный HTML и plain text. Push хранит зашифрованные device tokens с user/purpose binding; responses и логи их не возвращают. Email/push выключены по умолчанию. Текущие preferences и допуск повторно проверяются при вставке нового уведомления и перед отправкой; отключение recording category распространяется также на сохранённый Stage 5 fanout. Для внешней exactly-once доставки gateway обязан учитывать ключ идемпотентности; одной локальной записи недостаточно при потере сетевого ответа.

## 4 Календарь OAuth и идемпотентность

Create/update/cancel используют постоянный external ID, join URL и версию расписания. Product-only advisory lock сериализует внешнюю синхронизацию одной конференции; mapping сохраняется только при текущей аренде, версии и активном подключении. Gateway должен отклонять старый SourceVersion. Возвращённый после повторного create старый event обновляется до текущего расписания.

OAuth code/state — одноразовые и user/provider-bound, S256 PKCE, TTL 10 минут. Access/refresh/verifier шифруются AES-256-GCM независимым ключом. Callback и connect ограничены Redis, JSON строгий, frontend убирает query из адреса. Nginx не журналирует callback query в access/upstream-error logs; Referer отключён. Refresh не восстанавливает отозванный grant. Disconnect стирает локальные секреты, но не удаляет внешние события; неуспешный внешний revoke требует действий оператора.

## 5 Архитектура распознавания

Только strict-ready composite recording ставит STT job. Запись не ждёт распознавания и остаётся готовой при его отказе. Исторический backfill автоматически не запускается.

Worker читает приватный server-owned MinIO key, ограничивает MP4 по байтам и длительности и отдельно извлекает mono 16 kHz WAV. FFmpeg не получает произвольного URL и ограничен protocol whitelist, deadline и размером output. Временный каталог private, очищается на успехе и отказе. STT получает аудио потоком, не исходный огромный video. Worker проверяет отсутствие bucket policy при старте и readiness.

## 6 Модель расшифровки и API

`transcripts` хранит recording/conference, generation, status, language/provider, время и безопасную ошибку. `transcript_segments` хранит start/end milliseconds, ordinal, текст, nullable confidence и speaker label. Provider speaker ID очищается: диаризация не выдаётся за установление личности.

API возвращает nullable metadata, enabled/provider mode и canRetry; сегменты выдаются страницами до 200. Чтение следует live history policy для admitted joined/left участников. Retry доступен owner/co_host с cooldown и generation cap; concurrent revoke и дорогой POST сериализуются на conference row. Waiting/kicked/outsider не получают текст или право повторов.

## 7 Итоги ИИ и версии prompt

`meeting_summaries` сохраняет summary/key points/actions/topics, provider/model, prompt/schema versions и поколение исходного transcript. Prompt `meeting-summary-ru-en-v1`, schema `meeting-summary-v1`. Вход отделён от инструкций как untrusted data; tools и выполнение внешних действий не предоставляются.

Детерминированные Unicode chunks и ограниченный hierarchical reduce имеют общий deadline, input/output caps и concurrency. Неверная схема и ссылки на чужие сегменты не сохраняются. Assignee/date остаются null без явного буквального маркера в цитируемом тексте; это консервативная проверка, не доказательство всех смысловых связей. Для пустого transcript платный ИИ не вызывается. Ошибка ИИ не меняет transcript или recording.

## 8 Поиск и индексы

Generated `tsvector` + GIN индексируют title, segments и summary/search text синхронно с сохранением. Конфигурация `simple` поддерживает RU/EN токены без stemming, semantic/vector search отсутствует. Authorization входит в SQL до count/rank/page; soft-deleted записи и старое summary generation исключаются.

Ответ содержит plain-text snippet, conference/recording/transcript/segment IDs и timestamp, где применимо. Фильтры source/conference/UTC date, bounded query и pagination реализованы. Frontend не интерпретирует snippet как HTML.

## 9 Миграции базы

Добавлены `000015_background_jobs.up.sql`, `000016_external_integrations.up.sql`, `000017_recording_content.up.sql`. Таблицы: `background_jobs`, preferences/devices, calendar connections/OAuth states/mappings, integration deliveries, transcripts/segments, meeting summaries. Добавлены versioned conference triggers, ready trigger и GIN индексы.

Текущий migrator повторяет SQL при старте; новые миграции идемпотентны, повторное применение проверено. Прямого conference FK на jobs нет, чтобы ready-trigger не инвертировал established recorder locks. Product content FK используют RESTRICT, а не неожиданный каскад. Автоматические down migrations и framework истории миграций не добавлялись.

## 10 Очередь outbox и повторы

Очередь сама является транзакционным outbox: нет обязательного publish после commit. Claim атомарный, lease UUID и срок fenced, результаты дополнительно связаны с generation. Независимые bounded pools не блокируют HTTP или медиа. Финальная просроченная попытка фиксирует terminal error без нового платного вызова.

Retry только для transient ошибок, exponential backoff/jitter и ограниченный Retry-After. Операторский `PRODUCT_MAX_ATTEMPTS` ограничивает также старые trigger-created jobs. Ошибки сохраняются безопасными кодами, без response bodies. Media RabbitMQ очередь не переписана.

## 11 Проверка безопасности и приватности

Проверены JWT identity, UUID/JSON, IDOR, current admission/role checks, OAuth cross-user/provider/replay, encrypted tokens и revoke/refresh race, redirect refusal, private object paths, bounded extraction, prompt/schema validation и текстовый XSS. Callback/device mutations имеют Redis quotas; приватные ответы no-store/nosniff.

SQL logging отключён, provider error body не логируется, job payload не содержит полный текст или секреты. Query, IDs и URL не входят в metric labels. Подписанный video URL остаётся bearer доступом до короткого истечения; мгновенного отзыва уже выданного URL нет. Уже принятый внешним сервисом запрос не гарантированно отменяется при disconnect. Полный pentest и container OS CVE scan не выполнены.

## 12 Новые и изменённые файлы

Новые компоненты расположены в `cmd/product-worker`, `internal/domain/{jobs,integrations,content}`, `internal/usecase/{jobs,integrations,content}`, `internal/app/{content,integrations}`, `internal/infrastructure/{providers,contentproviders}`. Добавлены продуктовый bootstrap/metrics decorators, PostgreSQL job/integration/content repositories, private audio storage/extractor, AES token cipher, transport routes/tests и Stage 7 config/tests.

Frontend: новые `IntegrationsSettings`, `RecordingInsights`, `SearchPage`, helpers, unit/mock/live e2e tests и `playwright.integrations-live.config.ts`; обновлены API/types/router/layout/recording/notifications/styles. Инфраструктура: три миграции, product Dockerfile, `docker-compose.product-test.yml`, обычный/production Compose, env examples, CI, Makefile, Prometheus/alerts и Nginx OAuth privacy rules. Документация: README, это руководство и отчёт.

Existing API DI, notification generator/payload и несколько baseline lint/test строк изменены точечно. В Stage 5 chat test добавлена настройка адреса тестового MinIO, чтобы регрессию запускать изолированно. Пользовательский staged `.pyc` сохранён без изменений. Commits, push, PR и deployment не выполнялись.

## 13 Результаты тестов и проверок

`go test ./...`, `go test -race ./...`, `go vet ./...`, staticcheck 2026.1 v0.7.0 — PASS. Форматирование и `git diff --check` — PASS. Govulncheck v1.8.0: 0 в вызываемом коде, 0 в импортируемых пакетах; один module-only GO-2026-5932 относится к неиспользуемому deprecated OpenPGP, fixed version отсутствует. Это не утверждение об отсутствии всех CVE во всех зависимостях.

Frontend: typecheck, 101 unit tests, production build и production dependency audit — PASS, audit 0 vulnerabilities. Chromium UI: 11 PASS, 6 opt-in сценариев штатно пропущены в обычном запуске; Stage 7 real Docker acceptance запущен отдельно и PASS. Firefox не считается проверенным: локальный Playwright завершился до открытия страницы с ошибкой папки профиля.

## 14 Результаты тестовых провайдеров

Изолированные PostgreSQL integration tests подтвердили schedule/email/push/calendar create/update/cancel/dedup, future/stale reminders, fanout 205 участников с continuation, provider failure isolation, полный server OAuth/S256/encryption/refresh/revoke, recording ready/rollback → STT → AI → notifications, RU/EN FTS/timestamp/permissions, reprocess cooldown/lease/role-revoke fencing и повторные миграции. Fake HTTP проверяет 429/5xx/permanent отказ, redirects и неверную AI schema; настоящий FFmpeg extraction проверен.

Сквозная Docker проверка использует только mock STT/AI/calendar. Два Chromium участника действительно декодировали синтетические SFU frames, создан настоящий MP4, worker извлёк аудио и асинхронно сохранил демонстрационный текст и итоги. Участник открыл результат поиска, воспроизвёл MP4 и перешёл кнопкой сегмента к 0. Ненулевая метка 42,5 секунды покрыта unit, не этим mock STT. Постороннему запрещены recording/transcript/segments/summary, search пустой. Настоящие vendor API и платные вызовы не выполнялись.

Артефакты последнего запуска: `frontend/test-results/stage7-live/.../acceptance.json` и `stage7-summary.png`; JSON содержит точные UUID, не токены. Тестовые конференции закрыты; fixtures сохранены только в томах изолированного стенда.

## 15 Регрессия этапов 3 4 5 6

С реальными PostgreSQL/Redis/RabbitMQ/MinIO и race проверены authenticated WS/SFU, два пользователя, camera/screen layouts, compositor MP4/preview/mixed audio, lease recovery/fencing/auto-stop, chat/files/read races, waiting/history/moderation и notification dedup. Дополнительно PASS reconnect burst 100 WS и две параллельные полноценные записи. Общий выбранный regression запуск занял около 85 секунд.

Readiness API/media/recorder/product-worker на изолированном Compose — 200; health unit checks и закрытые metrics проверены. Новые внешние операции не включены в RTP/media hot path. При этом нового контролируемого before/after latency benchmark, 30-минутного soak, WAN/TURNS и полного failure-injection набора Stage 6 не проводилось; отсутствие роста latency нельзя объявлять измеренным производственным результатом.

## 16 Метрики и эксплуатация

Добавлены bounded job outcomes/durations/backlog, provider call attempts/durations/rate-limit class и search outcomes/latency. Метрики не измеряют token usage или денежную стоимость: gateway contract их не возвращает. Prometheus включает `product-worker:8092`; добавлены starter backlog/failure/rate-limit alerts. Promtool проверил конфиг и все 16 правил, Nginx синтаксис проверен для frontend, HTTPS и production example.

Docker образа API/media/recorder/product-worker/frontend собраны. Stage 7 overlay и production Compose syntax проверены без production secrets. Изолированный стек использовался вместо основного локального проекта; основной Compose не перезапускался. Ресурсные пределы tmpfs, RAM, concurrency и operator retry описаны в руководстве. Автоматический retention purge не внедрён.

После проверки изолированные тестовые контейнеры остановлены, тома и тестовые данные сохранены. API, recorder, media-worker и product-worker завершились с кодом 0; остановка coturn и RabbitMQ закончилась кодом 137. Основной локальный стек продолжает работать без перезапуска.

## 17 Известные ограничения

Generic gateway требует реальной реализации и provider-side idempotency/version fencing; качество живого распознавания, ИИ, email/push доставки и конкретных OAuth scopes пока не проверено. Email verification у проекта отсутствует; перед публичной рассылкой требуется отдельное решение. Browser web-push SDK/service worker не добавлены. Автоматическое раскрытие адресов attendees и bidirectional calendar edits отсутствуют.

FTS без морфологии и семантики; summary evidence rule консервативен и не заменяет человеческую проверку. Есть только текущие поколения текста, не неизменяемый архив всех версий. Нет cluster-wide provider budget, общей temp disk quota, automatic SIGKILL-orphan cleanup или automatic text/job/audit TTL. Soft deletion скрывает текст, но не удаляет его физически или из backups. Перед production нужны согласованные retention/consent, live sandbox и целевая ёмкость.

## 18 Рекомендуемые приоритеты этапа 8

Сначала review Stage 7 на целевом staging: выбранный provider gateway, scopes/consent, реальные RU/EN записи, расходы, retention/backups и Firefox. После согласования можно планировать live captions/transcription и расширенные режимы записи. Semantic/vector search имеет смысл только после измерения недостаточности FTS; meeting analytics — отдельная следующая задача. Автономные действия, внешние задачи и billing в этом этапе не добавлены. Этап 8 автоматически не запускается.
