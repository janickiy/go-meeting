# Release notes — шаблон

Скопируйте этот шаблон в отдельный документ выпуска. Значения «не проверено»
не заменяются на «пройдено» без evidence; пропущенная проверка получает причину,
риск и ответственного. Не включать secrets, JWT, реальные тексты встреч и signed URL.

## Идентификаторы

| Поле | Значение |
| --- | --- |
| Версия | `vX.Y.Z` |
| Полный commit SHA | Не заполнено |
| Build UTC / source SHA-256 | Не заполнено |
| Release manifest / SHA-256 | Не заполнено |
| Registry digests / SBOM / scan | Ссылки на артефакты |
| Source dirty | Должно быть `false` для registry/staging/production |
| Предыдущий manifest | Не заполнено |
| Ответственные / окно | Не заполнено |

## Пользовательские изменения

- Добавлено: описать фактическое поведение без внутренних названий этапов.
- Исправлено: видимый пользователю эффект.
- Изменено/удалено: совместимость и необходимое действие пользователя.
- Известные ограничения: браузеры, сети, providers и сценарии, которые не подтверждены.

## Операторские изменения

- Config/feature flags: новые значения, безопасный default, потребители, нужен ли restart.
- Секреты/providers: способ безопасной подготовки/ротации, не сами значения.
- Schema: новые migration filenames/checksums, измеренные duration/locks, Expand/Contract.
- Data: backfill, retention/удаление, изменение формата объектов или content.
- Dependencies: обновления image digests/Go/npm и отдельное maintenance-окно, если нужно.
- Capacity: среда, сценарий, число операций, длительность и ограничения измерений.

## Проверки

| Проверка | Статус | Evidence / среда / дата |
| --- | --- | --- |
| Validate / unit / race / integration | Не проверено | |
| Frontend lint / typecheck / unit / a11y / build / CSP | Не проверено | |
| Chromium / Firefox / ручная доступность | Не проверено | |
| Dependency/image scan / SBOM | Не проверено | |
| Local immutable rehearsal | Не проверено | Не считается staging |
| Настоящий staging HTTP/auth/WS | Не проверено | |
| Media / forced relay / TURNS / запись / preview | Не проверено | |
| Backup / isolated restore-check / бизнес-проверка данных | Не проверено | |
| Application rollback на текущей схеме | Не проверено | |
| Production smoke / окно наблюдения | Не проверено | |

## План запуска и отката

1. Выбранный immutable manifest, target env/project и разрешённые команды.
2. Write freeze, backup, migration, deploy; кому сообщают о maintenance.
3. Pilot/flags: глобальный набор для deployment либо отдельный изолированный
   стенд; не указывать cohort targeting, которого нет в приложении.
4. Наблюдение: окно, фактические traffic counts, SLI и согласованные stop conditions.
5. Previous manifest и доказательство совместимости старого кода с новой БД/config.
6. Если rollback несовместим: согласованный forward fix/DR-план, точка данных,
   допустимые потери и ответственный. Автоматический DB down запрещён.

## Решение

Статус: **не согласовано**. Дата/время, принимающий решение, ссылки на evidence,
исключения и их срок — заполнить перед promotion. Отдельно указать, что остаётся
неподтверждённым. Структура machine-readable approval —
[RELEASE_PROCESS.md](RELEASE_PROCESS.md); полный checklist —
[PRODUCTION_LAUNCH_CHECKLIST.md](PRODUCTION_LAUNCH_CHECKLIST.md).
