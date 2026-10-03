# Выпуск неизменяемых артефактов

Цепочка выпуска: исходники → validate/test/security → сборка → integration →
scan/SBOM/package → staging → smoke/наблюдение → решение оператора → production →
проверка либо откат. Реальный staging пока отсутствует; соответствующие evidence
остаются неподтверждёнными. Локальные результаты не переводят их в `true`.

## Версия, артефакты и CI

Политика тегов: `vX.Y.Z`, допускается prerelease вроде `v1.2.3-rc.1`.
Major отражает несовместимые контракты, minor — совместимые возможности,
patch — совместимые исправления; даже patch отдельно оценивается по SQL и config.
Release notes составляются по [шаблону](RELEASE_NOTES_TEMPLATE.md).

Каждый выпуск связывается с полным Git SHA, UTC-временем и fingerprint исходников.
Registry release требует чистого source; package сверяет version/commit и source
fingerprint с OCI labels, а staging/production требуют
`sourceFingerprintVerified:true`. `--allow-dirty-local` допускается только
для локальной репетиции и фиксируется `sourceDirty:true`. Это не production-релиз.
Публичные `/version` и `/version.json` сверяются с манифестом, а не с mutable tag.

Текущий [GitLab pipeline](../.gitlab-ci.yml):

| Gate | Проверки |
| --- | --- |
| Validate | gofmt, vet, staticcheck, синтаксис release-скриптов |
| Test | Go unit/race; frontend lint, TypeScript, unit, a11y, production build, CSP |
| Security | govulncheck; npm audit, блокирующий high/critical |
| Build / integration | Бинарники, PostgreSQL/Redis integration, Chromium и Firefox E2E с включённым privacy-тестом |
| Package тега | Docker build, SBOM и scan всех release-образов; high/critical блокируют пакет; публикация по digest |
| Тяжёлые сценарии | SFU/load/soak manual или scheduled; media/recording/TURN smoke обязателен для соответствующих изменений независимо от общего CI |

`release-package` требует отдельного Docker runner, registry credentials и
проверенного `TRIVY_IMAGE=…@sha256:…`. Подключение этой инфраструктуры ещё должно
быть подтверждено. Текущий `deploy-dev` вызывает старую синхронизацию/пересборку
и не является production promotion. CI сам не принимает бизнес-решение о запуске.

## Сборка и упаковка один раз

Примеры выполняются из корня checkout с подготовленными инструментами и
Docker. Каталоги/версии выбираются новыми; существующий release tag не перезаписывается.

```bash
RELEASE_VERSION=v1.2.3
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_DIRECTORY=/absolute/artifacts/v1.2.3
bash scripts/release/build.sh --version "$RELEASE_VERSION" --commit "$RELEASE_COMMIT" --registry registry.example.org/team/meet --output "$ARTIFACT_DIRECTORY/build"
bash scripts/release/package.sh --build "$ARTIFACT_DIRECTORY/build/build.json" --output "$ARTIFACT_DIRECTORY/package" --mode registry
```

Это bootstrap-пакет для первого развёртывания. При обычном обновлении приложения
добавьте к команде упаковки `--infrastructure-manifest
/absolute/artifacts/previous/release.json`. Пакет сохранит **точно те же** образы
MinIO, PostgreSQL, Redis, RabbitMQ, Coturn, proxy, Prometheus и Grafana; новыми
будут только шесть служб приложения. Отдельно собранный новый MinIO в этом
случае не используется. Предыдущий манифест должен быть проверен оператором и
иметь тот же режим (`local` или `registry`). Его SHA-256 записывается в новый
манифест, а security scan повторяется и для сохранённой инфраструктуры.

В CI передайте предыдущий манифест через защищённую File variable
`RELEASE_INFRASTRUCTURE_MANIFEST`; job автоматически добавит этот флаг. Только
для первого развёртывания оставьте File variable пустой и явно задайте
`RELEASE_BOOTSTRAP=true`. Без выбранного режима job останавливается до сборки;
одновременно включать оба режима нельзя. Автоматически выбирать «последний» tag или менять инфраструктуру
вместе с кодом приложения запрещено. Изменение любого инфраструктурного digest
оформляется отдельным обслуживанием с backup, проверкой совместимости данных и
собственным планом отката. Обычный `release.sh deploy` такое изменение отклоняет.

Адрес registry в примере нужно заменить. Учётные данные Docker login получают
из secret store/CI, пароль передают через stdin, не аргумент командной строки.
`TRIVY_IMAGE` задаётся до package и должен быть закреплён digest. Флаг frontend
телеметрии `CLIENT_TELEMETRY_ENABLED=true` задаётся перед build при явном решении
его включить; по умолчанию отправки нет.

Для локальной репетиции: собственный prefix, например `meet-release-local`,
при необходимости `--allow-dirty-local` у build, `--mode local` у package.
`--skip-security-local` разрешён только локально и означает **непроверенный**
пакет. Local package сохраняет image ID, `images.tar`, `release.json`,
`SHA256SUMS`; registry package сохраняет registry digests, scan и SBOM.
Неполную сборку после ошибки не считать готовым пакетом и не дописывать поля
`securityScanned` вручную. Неизменяемость манифеста не делает неизвестный
источник автоматически доверенным.

Операторские бинарники тоже готовятся заранее в CI/на build-host и проверяются
для архитектуры машины запуска. Например `go build -trimpath -o
/absolute/artifacts/bin/release-smoke ./tools/release-smoke`; выполняйте это до
release, не при аварийном rollback. Образы включают служебные migration/backup
бинарники. При переносе local archive сначала сверяют `SHA256SUMS`, затем
загружают архив Docker; во время инцидента старый commit не пересобирают.

## Миграции

Один назначенный оператор/CI job выполняет `release.sh migrate` **до** запуска
нового приложения. В production Compose `AUTO_MIGRATE=false`: старт проверяет
ledger, но не выполняет DDL. `release_schema_migrations` содержит имя файла,
SHA-256 точных байтов, время применения и duration. Применённый SQL нельзя
переименовывать, переводить комментарии или редактировать: добавляется новая миграция.

Migration job сериализован PostgreSQL advisory lock, использует транзакцию,
`lock_timeout=15s` и общий deadline 5 минут. При ошибке batch откатывается и
promotion прекращается. Старая БД без ledger один раз проходит имеющиеся
идемпотентные up-файлы; слепое заполнение ledger не предусмотрено. Измеряйте
длительность и блокировки на представительной копии, включая индексы миграции
`000021`. Не переносите короткий локальный замер на большую production-БД.

Рискованные изменения: Expand → совместимый deploy → backfill/verify → Contract
в отдельном последующем выпуске. Rollback приложения не выполняет DB down.
Ledger допускает более новые миграции, но не доказывает семантическую совместимость
старого кода; она проверяется на rehearsal и описывается в release notes.

## Проверка и продвижение

На staging выполнить подготовку baseline, write freeze/backup, migration,
deploy нового пакета, smoke/E2E, forced relay/TURNS и запись с ffprobe/preview/
MinIO, проверку dashboards, откат на прежний пакет и повторный deploy текущего.
Сохранять результаты, времена, версии, ограничения и отказы. Процедура backup:
[backup-restore.md](operations/backup-restore.md); команды deploy —
[DEPLOYMENT.md](DEPLOYMENT.md).

Smoke использует заранее проверенный бинарник и отдельный non-admin аккаунт:

```bash
bash scripts/release/smoke.sh --environment staging --project recorder-staging --env-file /srv/meet/config/staging.env --manifest /srv/meet/releases/v1.2.3/release.json --base-url https://meet-staging.example.org --binary /srv/meet/tools/release-smoke --exercise
```

`SMOKE_EMAIL`/`SMOKE_PASSWORD` поступают из защищённого окружения. `--exercise`
создаёт отдельную встречу, проверяет join/WS snapshot и завершает участие/отменяет
встречу; записи тестового сценария сохраняются для адресного осмотра. Production
smoke запрещает `--exercise` и не создаёт конференцию. Проверка включает версии,
frontend HTTP, login, auth/me, capabilities, ICE config и запрет admin; она сама
не проверяет реальный RTP, TURN, FFmpeg, файлы, waiting room и полный browser UX.

## Approval production

Для каждой изменяющей production-операции нужен существующий абсолютный
`--evidence` JSON и `RELEASE_PRODUCTION_APPROVAL`, точно равный версии выбранного
манифеста. Evidence должен описывать **реальные** проверки и содержать:

```json
{
  "version": "v1.2.3",
  "commit": "FULL_40_CHARACTER_SHA",
  "manifestSHA256": "SHA256_OF_EXACT_RELEASE_JSON",
  "migrationCompatible": false,
  "backupVerified": false,
  "rollbackReady": false,
  "stagingSmokePassed": false,
  "mediaSmokePassed": false,
  "securityPassed": false,
  "applicationRollbackCompatible": false,
  "approvedBy": "",
  "evidenceReferences": []
}
```

Этот пример намеренно не проходит gate. Для deploy/migrate/drain/resume нужны
первые шесть подтверждений совместимости/backup/rollback/staging/media/security;
для rollback вместо трёх последних проверяется `applicationRollbackCompatible`.
`approvedBy` и `evidenceReferences` обязательны. Commit и hash манифеста должны
совпасть побайтно. Скрипт проверяет структуру утверждений, не истинность подписанта
и не подлинность стороннего отчёта; проверка evidence — ответственность оператора.

```bash
export RELEASE_PRODUCTION_APPROVAL=v1.2.3
bash scripts/release/release.sh deploy --environment production --project recorder-production --env-file /srv/meet/config/production.env --manifest /srv/meet/releases/v1.2.3/release.json --evidence /srv/meet/evidence/v1.2.3.json
unset RELEASE_PRODUCTION_APPROVAL
```

## Поэтапное включение и наблюдение

Встроенного targeting по пользователям/accounts и traffic-splitting canary нет.
Флаги конфигурации действуют на развёртывание. Реализуемый минимум: проверить
выпуск в отдельном изолированном окружении/пилотном deployment, затем вручную
включить согласованный набор функций в целевом env и пересоздать совместимые
процессы. Не направляйте разные экземпляры одного сервиса на несогласованные
флаги или provider contracts. Выключение флага не удаляет ранее созданные данные.

После deploy проверить HTTP/auth/WS, media join/ICE/TURN, завершение записей,
backlog Rabbit/PG jobs, БД/Redis/MinIO, provider errors и клиентские ошибки.
Предлагаемый порядок наблюдения и SLI — [runbook](operations/README.md).
Системные ошибки auth, media/WS, повреждение/недоступность записи, несовместимость
схемы или неконтролируемый рост ресурсов блокируют продвижение и требуют
решения об отключении вспомогательной функции либо [rollback](ROLLBACK.md).
