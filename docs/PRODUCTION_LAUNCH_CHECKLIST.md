# Решение о запуске production

Это текущий go/no-go checklist, а не отчёт об успешном запуске. Реальная staging
и production-инфраструктура ещё не предоставлена. Все пункты ниже намеренно
не отмечены; к каждому подтверждению нужны дата, ответственный и ссылка на evidence.
Локальная автоматизация, unit/E2E и isolated rehearsal фиксируются отдельно
и не закрывают WAN, TURNS, реальные данные и capacity.

## Ответственность и артефакт

- [ ] Назначены владелец выпуска, оператор, принимающий rollback-решение и контакты инцидента.
- [ ] Утверждены release notes, scope, известные ограничения и окно обслуживания.
- [ ] Registry release собран из clean commit; версия/SHA/time/source hash совпадают с manifest.
- [ ] Все image references неизменяемы, checksum manifest проверен; предыдущий пакет доступен без пересборки.
- [ ] CI validate/unit/race/integration/frontend lint/typecheck/unit/a11y/build/CSP прошёл для этого commit.
- [ ] Browser E2E прошёл в Chromium и Firefox; средовые failures разобраны, а не скрыты skip.
- [ ] govulncheck/npm audit/image scan прошли, SBOM сохранены; исключения документированы и согласованы.
- [ ] Docker/registry/CI runner и `TRIVY_IMAGE` проверены; production не использует legacy `deploy-dev`.

## Инфраструктура и безопасность

- [ ] Staging независим от production, DNS и HTTPS/WSS доступны с реальных клиентских сетей.
- [ ] TURNS доверен браузерами; UDP/TCP/relay firewall проверен, выбранная ICE pair действительно relay при forced test.
- [ ] SFU public IP/NAT/mux и TURN relay range согласованы с Compose/firewall; проверены корпоративная и мобильная сети.
- [ ] PG/Redis/Rabbit/MinIO/private control/metrics/pprof не доступны публично; admin требует серверное право.
- [ ] Origins/CSP/CORS/trusted proxy настроены точно; IP rate limit различает клиентов и не доверяет произвольному forwarding.
- [ ] Все secrets независимы, вне Git/образов, файлы защищены; provider encryption key включён в защищённый recovery-план.
- [ ] Ротация JWT/service/metrics/TURN/provider secrets проверена; logout не ошибочно считается отзывом всех JWT.
- [ ] Диски/spool/volumes, права UID 1000, резерв места, CPU/RAM и FD соответствуют измеренной нагрузке.
- [ ] Принят риск single-host SPOF и отсутствия бесшовной миграции живых комнат; план outage понятен.
- [ ] Доставка alert ответственному проверена: наличие правила в Prometheus не считается доставленным уведомлением.

## Схема и данные

- [ ] `AUTO_MIGRATE=false`, migration запускается явно один раз до deploy; ledger/checksums не переписаны.
- [ ] Миграции и блокировки измерены на репрезентативной копии; старый код проверен на новой схеме.
- [ ] Согласованный write freeze/backup выполнен; все писатели, uploads и очереди учтены.
- [ ] Restore-check выполнен на отдельном target; checksum/privacy/signed access и бизнес-связи проверены.
- [ ] Утверждены реальные RPO/RTO, срок хранения, защита off-host backup и решение о потерях при disaster recovery.
- [ ] Осознаны ограничения backup: текущие версии объектов/ContentType, без history/tags/произвольной metadata и broker/spool snapshot.
- [ ] Восстановленные тестовые БД/bucket и тестовые fixtures учтены для адресной последующей очистки.

## Продуктовые сценарии

- [ ] Отдельный non-admin smoke account проверяет login/me/capabilities и отказ admin; секреты не попадают в отчёт.
- [ ] Регистрация, вход/выход, профиль и подтверждённые ограничения восстановления пароля соответствуют USER_GUIDE.
- [ ] Создание, приглашение, prejoin, waiting room/admission, роли и reconnect прошли в браузерах.
- [ ] Камера/микрофон/screen share проверены на реальных устройствах, разрешениях, смене сети и нескольких участниках.
- [ ] Запись каждого включённого режима стартует/останавливается, ffprobe подтверждает media, MP4/preview доступны только разрешённым пользователям.
- [ ] Чат/вложения/history/download защищены, работают лимиты и истечение ссылок.
- [ ] STT/AI/live captions/search/analytics, если включены, проверены с реальным provider и отказами; mock не используется.
- [ ] Email/push/calendar/OAuth, если включены, проверены с реальными credentials, consent и revoke/refresh; по умолчанию остаются noop/disabled.
- [ ] Телеметрия, если включена, не передаёт текст ошибок/контент/URL с ID/токены; backend rate limit и opt-out проверены.
- [ ] Пользовательская доступность и адаптивность проверены вручную; автоматический a11y scan не объявлен полной сертификацией.

## Репетиция, запуск и наблюдение

- [ ] На настоящем staging пройдены migration → deploy → smoke/media → rollback → повторный deploy того же пакета.
- [ ] Проверен drain активных WS/комнат/recordings/jobs; timeout не вызывает скрытый force restart.
- [ ] До recorder drain прекращены новые `record.start`, обработаны pending starts и штатно завершены записи; учтена возможная задержка stop из-за requeue в общей очереди.
- [ ] Заполнено правдивое version-bound evidence, production approval равен выбранной версии.
- [ ] Согласован объём пилота: отдельный deployment либо глобальные флаги; не обещан несуществующий cohort targeting.
- [ ] До запуска выбраны наблюдаемые SLI, ожидаемый объём трафика и условия остановки; исторический baseline не принят за SLO.
- [ ] После запуска версии/health/auth/WS/media/recordings/очереди/storage проверены; фактические counts и окно наблюдения сохранены.
- [ ] Ответственный явно принял go/no-go; при no-go выполнен безопасный rollback/forward fix и уведомление.

## Evidence запуска

| Поле | Заполняется оператором |
| --- | --- |
| Версия / commit / hash manifest | Не заполнено |
| Staging URL / production URL | Не предоставлены |
| CI, scan, browser и media evidence | Не заполнено |
| Backup / restore-check / rollback rehearsal | Не заполнено |
| Начало / конец / число реальных операций наблюдения | Не заполнено |
| Решение / ответственный / время | **NO-GO до закрытия обязательных внешних gates** |

Процедуры: [DEPLOYMENT](DEPLOYMENT.md), [RELEASE_PROCESS](RELEASE_PROCESS.md),
[ROLLBACK](ROLLBACK.md), [backup/restore](operations/backup-restore.md),
[runbook](operations/README.md). Исторические отчёты этапов
[6](operations/production-hardening-report.md) и
[9](operations/PRODUCT_UX_RELEASE_REPORT.md), включая прежний
[UX checklist](operations/RELEASE_CHECKLIST.md), описывают прошлые проверки,
не заменяют текущую приёмку production.
