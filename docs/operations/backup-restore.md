# Резервная копия и проверка восстановления

Автоматизация сохраняет логический PostgreSQL dump и **текущие версии** объектов
одного настроенного MinIO bucket. Это не атомарный snapshot двух хранилищ:
согласованность требует остановить все записывающие операции на время копирования.
Успешный backup без restore-check не считается проверенным восстановлением.

## Что входит и что не входит

| Данные | Покрытие |
| --- | --- |
| PostgreSQL | Custom-format `pg_dump`, schema/data и migration ledger; без владельцев и ACL |
| MinIO | Текущий object key, байты, размер, SHA-256 и `ContentType`; приватность проверяется при восстановлении |
| Release | Копия `release.json`, его hash, версия/commit и время backup |
| История MinIO | Version IDs, старые версии/delete markers, tags и произвольная metadata не переносятся |
| Настройки инфраструктуры | IAM/ACL, lifecycle, replication, bucket policy как конфигурация, TLS, env/секреты и Grafana не архивируются этим helper |
| Realtime | Redis presence/leases/PubSub не восстанавливаются из старого состояния |
| RabbitMQ и локальный spool | Полный broker snapshot и незавершённые chunks не входят; требуют отдельного плана согласованного сохранения |

Для disaster recovery отдельно сохраняйте проверенные config, credentials,
ключ шифрования provider-токенов, TLS, инфраструктурные политики и нужные
операционные данные в защищённом хранилище. Дамп содержит личные данные, сообщения,
тексты расшифровок и зашифрованные provider-токены. Для чтения последних нужен
исходный `PROVIDER_TOKEN_ENCRYPTION_KEY`; потеря ключа не исправляется restore БД.
Срок хранения/удаления, RPO/RTO и доступ к backup утверждает владелец системы:
проект не устанавливает юридическую политику автоматически.

## Подготовка согласованного backup

1. Выбрать новый абсолютный каталог вне Git; защитить родительский каталог,
   проверить место и дальнейшее шифрование/доставку off-host. Helper использует
   `umask 077`, но не шифрует backup самостоятельно.
2. Зафиксировать установленный manifest и конфигурацию. Запретить параллельные
   deploy/migrate/maintenance на всё окно: backup и restore-check держат общий
   project lock во время своей операции, но не между отдельными drain/backup/resume.
3. Ограничить admission во внешнем proxy/LB, уведомить пользователей, завершить
   встречи/записи. Прекратить новые `record.start`, дождаться обработки ожидающих
   стартов и завершения записей, пока штатная stop-операция API ещё доступна.
   Только затем выполнить общий drain и проверить отсутствие активной работы.
   `active=0` не доказывает пустую очередь; recorder во время drain возвращает
   start с requeue/250ms backoff, что может задерживать stop в общей очереди.
   Composite stop сохранён в SQL, но его финализацию всё равно нужно проверить.
   Учитывать прямые записи в БД/MinIO, presigned uploads, внешние cron и админские
   операции — процессный drain не запрещает им писать.
4. Проверить завершение записи/финализации/uploads и фоновых jobs. Rabbit-команды,
   ожидающие исполнения, не считать сохранёнными этим backup. Не снимать freeze
   до успешного завершения обеих частей.
5. Только после этой проверки выставить `RELEASE_BACKUP_QUIESCED=1`. Переменная —
   утверждение оператора; скрипт не способен доказать глобальную остановку записи.

Пример staging (из корня репозитория):

```bash
INSTALLED_MANIFEST=/srv/meet/releases/v1.2.3/release.json
BACKUP_DIRECTORY=/srv/meet/private-backups/2026-10-03T120000Z
bash scripts/release/release.sh drain --environment staging --project recorder-staging --env-file /srv/meet/config/staging.env --manifest "$INSTALLED_MANIFEST"
export RELEASE_BACKUP_QUIESCED=1
bash scripts/release/backup.sh --environment staging --project recorder-staging --env-file /srv/meet/config/staging.env --manifest "$INSTALLED_MANIFEST" --output "$BACKUP_DIRECTORY"
unset RELEASE_BACKUP_QUIESCED
bash scripts/release/release.sh resume --environment staging --project recorder-staging --env-file /srv/meet/config/staging.env --manifest "$INSTALLED_MANIFEST"
```

`resume` выполняют после успешного backup, если нужно вернуть тот же выпуск;
при подготовке migration/deploy сохраняют согласованное maintenance-окно.
В production `drain` и `resume` дополнительно требуют version-bound approval и
`--evidence` из [RELEASE_PROCESS.md](../RELEASE_PROCESS.md). Сам `backup.sh`
проверяет target/manifest и quiesce-флаг, но не требует production approval:
это не отменяет организационное согласование окна и запрет конкурентных операций.
Все операторы должны использовать один `RELEASE_LOCK_DIR` на выбранном Docker-хосте.

Каждый PostgreSQL-подпроцесс ограничен `RELEASE_DATABASE_TIMEOUT` (по умолчанию
600 секунд), `pg_dump` дополнительно использует `--lock-wait-timeout=5s`.
Лимит применяется отдельно к createdb/restore/dump/ledger-check, а не суммарно
ко всему drill; объектная операция имеет свой deadline. При превышении helper
прерывается, не объявляя backup/restore успешным. Если лимит меняется, это должно
следовать из замера объёма данных и согласованного maintenance-окна.

Выход: `postgres.dump`, `objects/manifest.json` и файлы объектов, `release.json`,
`backup.json`. Последний содержит SHA-256 dump/manifest объектов и
`restoreVerified:false`; он не превращается автоматически в `true` при проверке.
Частичный каталог после ошибки не использовать как завершённую копию. Сохранить
его для диагностики либо удалить адресно после решения оператора.

## Изолированная проверка restore

Target — отдельное local/staging-окружение с работающими PG и MinIO, достаточным
местом, подходящей версией PostgreSQL и совместимым release manifest. Нельзя
направлять тест в production: helper это запрещает. Он создаёт новые БД и bucket,
не восстанавливает поверх рабочих. Наличие уже работающего staging само по себе
не означает, что его зависимости разрешено использовать для drill: назначьте
отдельный project/env с независимыми данными.

```bash
bash scripts/release/restore-check.sh --environment staging --project recorder-staging-restore --env-file /srv/meet/config/restore.env --manifest /srv/meet/releases/v1.2.3/release.json --backup /srv/meet/private-backups/2026-10-03T120000Z
```

Для локальной репетиции допустим `--environment local --project
recorder-release-rehearsal-restore` с собственным `.env.local` и отдельными
портами/хранилищами. `restore-check.sh` сам зависимости не поднимает: сначала
подготовить target согласно [DEPLOYMENT.md](../DEPLOYMENT.md), исключив пересечение
портов с другими стендами.

Проверка выполняет:

- Проверку hash dump, object manifest и скопированного `release.json` по
  `backup.json`, а также совпадение версии/commit исходного manifest.
- `pg_restore --exit-on-error --single-transaction` в новую БД;
  полный повторный dump для чтения каталога и непустой `release_schema_migrations`.
- Восстановление объектов в новый bucket без перезаписи существующего key,
  проверку размеров/checksums и приватной bucket policy.
- Для непустого bucket — чтение первого объекта по signed URL и запрет anonymous
  GET. Для пустого bucket `signedAccessVerified:false`, это явно не проверенный download.

Отчёт `restore-check-<timestamp>_<pid>.json` сохраняется рядом с backup. Он
содержит hash исходного и целевого release manifest, имена созданных БД/bucket,
числа объектов/миграций, длительности этапов
и результаты проверок. `productionRPO`/`productionRTO` остаются `null`: локальные
секунды — не гарантия восстановления production. Не меняйте исходный backup
manifest, чтобы добавить неподтверждённые утверждения.

Тестовые БД и bucket **сохраняются**, в том числе при частичном отказе. Осмотрите
их и отчёт, затем удаляйте только записанные точные имена после согласования.
Автоматической очистки по prefix и команды удаления всех volumes нет.

## Пределы проверки и аварийное восстановление

Hash подтверждает целостность относительно manifest, но не доверенность его
источника: хранить защищённый manifest отдельно от потенциально изменяемой копии.
Restore-check не проверяет все бизнес-связи между PostgreSQL и object keys, не
запускает восстановленное приложение на временной БД и не воспроизводит каждую
запись. После drill отдельно сопоставить файлы/records/chats и проверить доступ,
просмотр MP4/preview, авторизацию и отсутствие утечек между пользователями.

Для реального восстановления оператор утверждает точку данных и допустимую
потерю изменений, сохраняет текущее повреждённое состояние для разбора, готовит
новый изолированный target, восстанавливает PG/объекты/config/ключи, проверяет
приложение и только затем переключает трафик. Broker replay/повторы jobs и
realtime leases рассматриваются отдельно. Копия PG без согласованных объектов
и наоборот не даёт согласованного восстановления. Нельзя применять тренировочную
команду как скрытый destructive restore поверх production.
