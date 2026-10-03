# Сканирование контейнеров: локальная репетиция

Этот отчёт сохраняет результат исходного комплекта. Исправленные 13 образов
и закрытая проверка удалённого стенда описаны в
[отдельном отчёте развёртывания](meeting-host-deployment.md); результат исходных image IDs не изменялся.

Дата: 3 октября 2026. Scanner [Trivy 0.75.0](https://github.com/aquasecurity/trivy/releases/tag/v0.75.0), digest `aquasec/trivy@sha256:af6acf9a6b85dfe389a1941505c0ce9efef52a4719635e1a962f022a3d855daa`.
DB UpdatedAt: `2026-10-03T07:01:46.027466673Z`; DownloadedAt: `2026-10-03T11:53:02.89709638Z`.

**HIGH/CRITICAL gate: FAIL. Production: NO-GO для проверенного комплекта.** Локальная репетиция с явным пропуском security gate не является production approval. Не добавлены ignore-all, ignore-unfixed или глобальные исключения CVE.

Проверены исходные локальные immutable image ID. Основные Go службы собраны с версией `v1.0.0-rehearsal.20261003.1`. Frontend и nginx в исходном наборе используют старую 1.27; результаты отдельного нового candidate 1.30.5 и финального frontend приведены ниже. Счётчики — число package findings, поэтому один advisory может встречаться в нескольких бинарниках и пакетах.

| Служба | Image ID | HIGH | CRITICAL |
| --- | --- | ---: | ---: |
| api | `sha256:a739902241dce94a75014e308bb34691f5efe0f1f7ff999c7fb34a3318db7c87` | 0 | 0 |
| frontend | `sha256:7ade6bed04227c3f1f3d79fa84e2aa42c7b7e786e96585837e1d5458fe9b05ed` | 42 | 2 |
| grafana | `sha256:74144189b38447facf737dfd0f3906e42e0776212bf575dc3334c3609183adf7` | 121 | 10 |
| live-worker | `sha256:9e522a142a8034cfa2afa965937ff8cc22f0bad7db4293b0cf40f149719724a8` | 0 | 0 |
| media-worker | `sha256:5f07ebbf4930d2c8d362ec709c579a7fdef6926b12e0a442f32f5e38969506c4` | 0 | 0 |
| minio | `sha256:eb0e194df2964c5307eaf96a5ff5583fd0bae39ae468e2fdc93ca1328ad50d91` | 39 | 6 |
| nginx | `sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10` | 42 | 2 |
| postgres | `sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777` | 30 | 1 |
| product-worker | `sha256:335baea826933895d4fb93187cd2e6ba6803100a0615daa639450f6ee056acb8` | 0 | 0 |
| prometheus | `sha256:63805ebb8d2b3920190daf1cb14a60871b16fd38bed42b857a3182bc621f4996` | 92 | 4 |
| rabbitmq | `sha256:b8e09bc63811e48c6621961b245da493fb5a116bd4424f810024362db1c5891b` | 0 | 0 |
| redis | `sha256:e7723ff73d963f5cc6d9c4643ea3d989527a402a319239054e9472a7fb9219a2` | 6 | 0 |
| worker | `sha256:bdf60f75478f851e42797924c23fb88c6bc84b6e6fa68c3a2e738516ee840f16` | 0 | 0 |
| coturn | `sha256:5b1932971b0573eb47c881ce1979b558ca6109b586ff5c79de7772ea37609f3c` | 130 | 11 |

Coturn `coturn/coturn@sha256:bbefd3e1fdfdc0d58770fe01b581fd8b00d9f3a5580d00acb77cf719a6bc78e3`: local Docker export сначала не прочитан Trivy из-за отсутствующего layer blob в tar. Повторное сканирование через registry по этому точному digest завершилось успешно; его результат включён в таблицу (Debian 13.6, 126 packages). Digest манифеста и Image ID конфигурации — разные идентификаторы.

## Повторный scan исправленного frontend/runtime

Сканирование той же версией scanner и DB, 3 октября 2026 в 12:04 UTC; новые образы приложения имеют версию `v1.0.0-rehearsal.20261003.2`.

| Артефакт | Image ID | HIGH | CRITICAL |
| --- | --- | ---: | ---: |
| api, финальный | `sha256:dbaa97c19550bba8775cbc238bc23b687cc8620318d057c7600110a22a07615b` | 0 | 0 |
| frontend, финальный | `sha256:d8d293383b1a9f18f19ba9323b60024fc952ed9421149fdc64806932c4a15ed0` | 2 | 0 |
| nginx:1.30.5-alpine, proxy | `sha256:43d9d8c1f8968f09df8c1aa6c136ecc617e62d64fe4c0b24c97de4eb210bd973` | 2 | 0 |

Оставшиеся HIGH у frontend/proxy: `CVE-2026-93990`, libexpat `2.8.4-r0`, исправление `2.8.5-r0`; `CVE-2026-103111`, pcre2 `10.48-r0`, исправление `10.49-r0`. Переход на Nginx 1.30.5 снял исходные Nginx findings, но сам по себе не сделал image gate зелёным. Финальные API/frontend проверены отдельно; нулевые результаты исходных worker образов не приписываются автоматически новым image IDs.

Исходные JSON отчёты этой проверки: `/tmp/recorder-release-security.qlPxpC/{service}.json`. При упаковке релиза скопировать их в evidence directory; временный путь не является долговременным хранением.

## Приоритетные результаты

- MinIO server: 39 HIGH / 6 CRITICAL, включая CVE-2026-33322 (OIDC JWT algorithm confusion), CVE-2026-33419 (LDAP login), без fixed version в DB. Транзитивный amqp091-go также имеет CRITICAL, исправления указаны с 1.13.0. OIDC/LDAP/AMQP integrations MinIO в текущем Compose не настроены, но это не blanket exception всем находкам: нужен отдельный reachable/configuration review и поддерживаемый maintenance source.
- Grafana 12.2.0: 121 HIGH / 10 CRITICAL; собственный CVE-2025-41115 имеет исправления 12.0.7/12.1.4/12.2.2 по DB, другие компоненты требуют отдельной проверки. Запрет внешнего доступа снижает поверхность, но не даёт green security gate.
- Prometheus 3.5.0: 92 HIGH / 4 CRITICAL в prometheus/promtool, включая gRPC и stdlib. Сопоставить исправленный upstream release со всем набором компонентов, а не повышать одну библиотеку внутри чужой поставки.
- PostgreSQL: 30 HIGH / 1 CRITICAL. CRITICAL обнаружен в Go stdlib утилиты gosu, не в PostgreSQL SQL engine. Нужен review фактического использования, актуальный совместимый image и повторный scan.
- Старые nginx/frontend: 42 HIGH / 2 CRITICAL; OpenSSL CVE-2026-31789 описывает 32-bit обработку сертификатов. Образ здесь arm64, поэтому конкретная эксплуатация этой находки не подтверждена. Другие Nginx/OS findings остаются и обновление runtime до проверенного candidate необходимо.
- Redis: 6 HIGH в OpenSSL packages; фиксированные Alpine revisions находятся в JSON.
- Coturn: 130 HIGH / 11 CRITICAL в зависимостях Debian (в том числе liblmdb, libmariadb, libxml2 и Perl). Некоторые advisory описывают серверные или 32-bit пути, не доказанные достижимыми в этой конфигурации TURN. Нужны отдельный triage каждого исключения и повторный scan поддерживаемого образа; глобального исключения не создано.
- Go-приложения и RabbitMQ: 0 HIGH / 0 CRITICAL в проверенных image IDs. Это не утверждение об отсутствии LOW/MEDIUM, всех типов secret/config issues или достижимых уязвимостей.

Полный technical/current/latest/license review: [launch-security-review.md](launch-security-review.md). Сторонние major upgrades и автоматическая замена object storage не выполнялись.

## Воспроизведение

```sh
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v recorder-release-trivy-cache:/root/.cache/trivy \
  -v /absolute/evidence:/reports \
  aquasec/trivy@sha256:af6acf9a6b85dfe389a1941505c0ce9efef52a4719635e1a962f022a3d855daa \
  image --image-src docker --scanners vuln --severity HIGH,CRITICAL \
  --exit-code 1 --format json --output /reports/service.json VERIFIED_IMAGE_ID
```

Сканирование для инвентаризации было без `--exit-code 1`, чтобы собрать все образы, затем результаты сопоставлены с блокирующей политикой. Нулевой exit такого inventory-запуска не означает, что gate пройден.
