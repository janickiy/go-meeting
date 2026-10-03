# Проверки frontend перед выпуском

Проверка 3 октября 2026. Исходный lockfile:
`frontend/package-lock.json`, SHA-256
`54e0afa0732456570581787654a51b0f89564fb3549e70e327145a2c916db125`.
Версии зависимостей не менялись. Проверки выполнялись в отдельной временной
копии с тем же lockfile и установленными через `npm ci` пакетами, не в
`node_modules` рабочего проекта. Использован Node 24.19.0; Node 23 хоста не
соответствует `engines` и не использовался для финальной сборки.

## Фактические результаты

| Проверка | Результат и границы |
| --- | --- |
| Prettier / TypeScript / CSP | PASS для основной реализации frontend; изменённый SemVer smoke assertion отдельно прошёл Prettier и real UI E2E |
| Unit | 157 тестов, 32 файла — PASS |
| Автоматическая a11y | 6 тестов — PASS; не заменяет ручную проверку доступности |
| Chromium E2E | 19 PASS; 7 opt-in инфраструктурных сценариев пропущены в базовом UI-прогоне |
| Version/telemetry privacy E2E | 2 целевых теста PASS: version metadata и отправка без Referer/Authorization/Cookie |
| Firefox на этом хосте | BLOCKED до запуска тестов: `Could not find profile folder`; не выдаётся за PASS |
| Настоящий local Compose UI, baseline | Chromium: 1 PASS за 5.3s на `https://localhost:25482`, API `http://127.0.0.1:28085`, фактический build `v1.0.0-rehearsal.20261003.1` |
| Настоящий local Compose UI, текущий выпуск после rollback/redeploy | Chromium: 1 PASS за 5.0s на тех же isolated URLs; точное совпадение build `v1.0.0-rehearsal.20261003.2` |
| npm audit, включая dev | 0 известных уязвимостей всех уровней на момент запроса; не гарантирует отсутствие неизвестных проблем |
| Установка обновлений | Не выполнялась |

Firefox проверялся с revision 1543, соответствующим установленному Playwright
1.63.0: основной cache, отдельные доступные profile/TMPDIR и новая официальная
копия браузера в отдельном cache дали одинаковую ошибку до выполнения теста.
Профиль пользовательского Firefox не менялся. Linux CI содержит оба браузера,
но наличие job не считается фактически выполненной проверкой.

Настоящий UI smoke подтвердил регистрацию, отсутствие join до подтверждения
prejoin и ровно один join после него, старт/завершение конференции, историю,
уведомления, отказ admin в UI и HTTP 403. В каждом из двух прогонов созданы
отдельный smoke-аккаунт и завершённая встреча в isolated rehearsal;
строки сохранены для осмотра, не
удалялись из рабочих проектов. Этот тест не включает передачу RTP и запись.
Проверка version использует точное `MEET_STAGE9_EXPECTED_BUILD_VERSION`, если
оно задано; прежний `stage9-local-` fallback сохранён для старого локального стенда.

Для самоподписанного localhost-сертификата Chromium использовал отдельный
временный Playwright config с `ignoreHTTPSErrors`, разрешённым только при явном
loopback opt-in. Это исключение проверки UI, не результат проверки TLS. Проверка
цепочки/имени для release HTTP/WSS выполняется отдельно с `SMOKE_CA_FILE`;
системное доверие и пользовательские браузеры не изменялись.

## Размер сборки

Изолированная production-сборка основной реализации:

| Вариант | Основной JS, kB | gzip, kB |
| --- | ---: | ---: |
| Исходный baseline | 256.26 | 80.02 |
| Метки версии, telemetry выключена | 257.06 | 80.23 |
| Telemetry включена при сборке | 259.74 | 81.37 |

Разница gzip включённого варианта относительно baseline — около 1.69%.
Это локальные размеры конкретного прогона, не весь download страницы и не
повторный замер итогового registry-артефакта. Телеметрия выключена по умолчанию;
контракт, privacy и ограничения — [API](../API.md#ошибки-frontend).

## Зависимости и доступные обновления

`npm outdated --depth=0 --json --long --registry=https://registry.npmjs.org`
выполнен read-only. Exit 1 в этом случае означает найденные обновления, не
ошибку установки. Для всех 20 прямых пакетов `latest` дополнительно прочитан
из публичного registry на 2026-10-03T12:04:57.269Z.
`wanted` определяется текущим диапазоном package.json; здесь версии закреплены
точно, поэтому совпадают с current. `latest` — registry dist-tag, не обещание
совместимости и не автоматическое разрешение обновления.
Определения полей: [официальная документация npm outdated](https://docs.npmjs.com/cli/v11/commands/npm-outdated/).

| Прямая зависимость | Current / lock | Wanted | Latest | License metadata |
| --- | --- | --- | --- | --- |
| `@fontsource/inter` | 5.3.0 | 5.3.0 | 5.3.0 | OFL-1.1 |
| `@tanstack/react-query` | 5.104.0 | 5.104.0 | 5.104.1 | MIT |
| `lucide-react` | 1.49.0 | 1.49.0 | 1.51.0 | ISC |
| `react` | 19.3.0 | 19.3.0 | 19.3.0 | MIT |
| `react-dom` | 19.3.0 | 19.3.0 | 19.3.0 | MIT |
| `react-router` | 8.4.0 | 8.4.0 | 8.4.0 | MIT |
| `@playwright/test` | 1.63.0 | 1.63.0 | 1.63.0 | Apache-2.0 |
| `@testing-library/jest-dom` | 7.0.1 | 7.0.1 | 7.0.1 | MIT |
| `@testing-library/react` | 16.3.3 | 16.3.3 | 16.3.3 | MIT |
| `@testing-library/user-event` | 14.6.7 | 14.6.7 | 14.6.7 | MIT |
| `@types/node` | 26.6.3 | 26.6.3 | 26.6.4 | MIT |
| `@types/react` | 19.3.0 | 19.3.0 | 19.3.0 | MIT |
| `@types/react-dom` | 19.3.0 | 19.3.0 | 19.3.0 | MIT |
| `@vitejs/plugin-react` | 6.1.1 | 6.1.1 | 6.1.1 | MIT |
| `axe-core` | 4.13.0 | 4.13.0 | 4.13.0 | MPL-2.0 |
| `jsdom` | 30.1.1 | 30.1.1 | 30.1.1 | MIT |
| `prettier` | 3.9.9 | 3.9.9 | 3.9.9 | MIT |
| `typescript` | 7.0.2 | 7.0.2 | 7.0.2 | Apache-2.0 |
| `vite` | 8.3.1 | 8.3.1 | 8.3.2 | MIT |
| `vitest` | 5.0.3 | 5.0.3 | 5.0.3 | MIT |

Рекомендуемый следующий отдельный цикл, без изменения этого выпуска:

- `@tanstack/react-query 5.104.1`: рассмотреть patch после release notes и
  регрессии cache invalidation, logout, history и reconnect.
- `vite 8.3.2`: рассмотреть patch сборщика после сравнения build/CSP,
  version.json, lazy assets и browser smoke.
- `lucide-react 1.51.0`: minor обновление проверять по используемым иконкам,
  визуальным снимкам, размерам и доступным именам элементов управления.
- `@types/node 26.6.4`: patch типов проверять с поддерживаемым runtime.
  Сейчас сборка использует Node 24, а типы относятся к major 26: typecheck
  не должен ошибочно разрешать API, отсутствующие в целевом Node 24.
  Согласовать стратегию runtime/types отдельно, не менять major вслепую.

Для каждого изменения нужен отдельный review changelog, обновлённый lockfile,
npm audit, lint/typecheck/unit/a11y/build/CSP и браузерная регрессия. Найденные
latest сами по себе не являются security blocker. Автоматический
`npm audit fix --force` и массовый major upgrade не выполнялись.

## Лицензионная инвентаризация

Полный raw inventory: [npm-dependency-license-inventory.csv](npm-dependency-license-inventory.csv).
Источник — поля lockfile, а не классификация по названию или предположение
о лицензии. Указаны package/version/path, direct/scope, dev/optional, license,
resolved/integrity и доступные direct current/wanted/latest.

В lockfile 162 внешние package-path записи, включая 47 optional-записей разных
платформ; у 150 стоит `dev:true`, у 12 нет этой отметки. Это не число JS-модулей
в browser bundle: сюда входят типы, tooling и платформенные binaries.
Все 162 записи содержат поле license:

| License metadata | Записей |
| --- | ---: |
| MIT | 109 |
| MIT-0 | 2 |
| OFL-1.1 | 1 |
| Apache-2.0 | 28 |
| MPL-2.0 | 13 |
| BSD-2-Clause | 2 |
| BlueOak-1.0.0 | 1 |
| ISC | 3 |
| CC0-1.0 | 1 |
| BSD-3-Clause | 2 |

`@fontsource/inter` содержит OFL-1.1, `lucide-react` — ISC.
MPL-2.0 отмечен у axe-core и lightningcss с его платформенными пакетами,
используемыми инструментами разработки/сборки. Поле `dev` не является
юридическим заключением о поставляемом составе.

Inventory не проверяет побайтно LICENSE/NOTICE каждого tarball, не выбирает
лицензию проекта и не подтверждает совместимость условий распространения.
Перед внешней поставкой сверить фактически включённые JS/CSS/fonts/binaries,
сохранить требуемые notices по проверенным текстам лицензий и согласовать
неоднозначности. Контейнерные компоненты, Nginx/FFmpeg/MinIO и Go рассмотрены
отдельно в [security review](launch-security-review.md) и
[container review](container-security-review.md).

## Воспроизведение

На поддерживаемом Node, в отдельной рабочей копии с тем же lockfile:

```bash
npm ci
npm run lint
npm run typecheck
npm test
npm run test:a11y
npm run build
sh scripts/test-csp-config.sh
npm audit --json --registry=https://registry.npmjs.org
npm outdated --depth=0 --json --long --registry=https://registry.npmjs.org
VITE_CLIENT_TELEMETRY_ENABLED=true npm run test:e2e:all
```

Последняя команда требует доступных поддерживаемых браузеров; нельзя считать
Firefox пройденным только потому, что Chromium выполнился. Для real Compose
smoke нужны отдельный loopback-стенд, явный opt-in и ожидаемая версия.
Production go/no-go — [актуальный checklist](../PRODUCTION_LAUNCH_CHECKLIST.md),
не этот локальный отчёт.
