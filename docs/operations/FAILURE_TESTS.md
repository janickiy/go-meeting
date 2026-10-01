# Отказные проверки, 1–2 октября 2026

Проект `recorder-stage6`, отдельные containers/volumes/ports. Основной
`go-recorder` не останавливался. Инъекция выполнялась без живых пользовательских
комнат; это проверка dependency readiness/recovery, не доказательство zero-loss.

| Инъекция | Снятие ready, сек | Ready после start, сек | Что подтверждено |
|---|---:|---:|---|
| Redis stop/start | 2.512 | 0.923 | API 503→200 |
| RabbitMQ stop/start | 2.994 | 2.948 | API 503→200, publisher восстановлен |
| PostgreSQL stop/start | 2.256 | 1.766 | API 503→200, pool переподключается |
| MinIO stop/start | 3.081 | 0.916 | API 503→200, bucket проверка восстановлена |
| media-worker stop/start | 0.282 | 0.332 | Процесс/control ready восстановлен |
| recorder stop/start | 0.196 | 2.143 | Процесс/consumer ready восстановлен |

Время включает выполнение Docker stop/start и polling с шагом 200ms; не является
SLA. Зависимости выполнялись последовательно. Дополнительные сценарии:

- Own AMQP connection close: publisher reconnect, consumer reconnect, последующая
  команда дошла; request UUID сохранился. Poison JSON подтверждён в durable failed
  queue, исходная доставка ack только после publisher confirm.
- FFmpeg helper пишет 2 МиБ: сохраняется не более 64 КиБ; игнорирующий TERM
  дочерний процесс принудительно завершается приблизительно через 3s после cancel.
  Shell metacharacters передаются буквально, не выполняются.
- SFU join/shutdown race, transport failure/admission timeout cleanup,
  auth/session expiry и Redis bus failure проверяются existing unit/integration
  тестами. Клиентские browser Stage 3/4 проверяют recreate PC после disconnect.
- Recorder lease fencing, потеря egress, recovery closed chunk и conference finish
  auto-stop проверены настоящим Stage 4 recorder E2E. Failed не превращается в ready.
- Forced TURN UDP/TCP + reconnect: выбран local relay candidate и двусторонний
  audio/video RTP. Остановка Coturn во время активной relay-only конференции:
  оба транспорта учитываются как failed, peers/rooms освобождаются (PASS, весь
  сценарий 24.94s, не время восстановления). После start Coturn повторный UDP/TCP
  RTP+reconnect PASS (8.27s). Обнаружен и исправлен пропуск failure counter при
  закрытии disconnected watchdog-ом раньше callback-а failed.
- External packet loss, disk-full, process kill -9 в середине chunk и outage
  storage во время большого upload пока не проверены этим прогоном. Это отдельные
  эксплуатационные риски.

## Эффект для пользователей и действия оператора

| Сбой | Пользовательский эффект | Потеря данных / восстановление |
|---|---|---|
| Redis | WS/presence/ownership перестают быть надёжными; clients disconnect | PubSub события не durable; после reconnect HTTP snapshot и история из PG. Старые leases не восстанавливать |
| Rabbit | Start/stop может откладываться или возвращать ошибку | Подтверждённые persistent сообщения остаются в брокере; возможны дубли. Проверить consumers/backlog/failed queue перед replay |
| PG | Auth/conference/chat/recording state операции не проходят | Не подтверждать mutation как success. Вернуть PG/лимиты, проверить locks и транзакции, при повреждении restore |
| MinIO | Upload/download/finalization недоступны, ready не ставится | Retained chunks и PG состояния сохраняются; token-prefix cleanup best effort, orphan сверяются после возврата storage |
| media-worker | RTP прерывается | Live room migration нет; новый ticket/PC/reconnect, незаписанный RTP потерян |
| recorder | Запись/финализация прерывается | Закрытые chunks пригодны для recovery; незавершённый хвост может быть утрачен. Проверить ownership, disk и leases |
| TURN | Relay клиент не собирает/не поддерживает маршрут | Новое подключение или reconnect после восстановления; media continuity не гарантируется |
| FFmpeg/disk-full | Recording failed, не success | Снять admission, сохранить валидные chunks, освободить/расширить диск после сверки данных; причина и status должны быть видны |

Production restart сначала drain/LB removal, затем SIGTERM с достаточным grace
period. Отдельный рестарт readiness-проверки не доказывает успешную финализацию
всех существующих записей. Для этого нужна живая recording failure matrix на
целевом сервере и согласованные критерии допустимого tail loss.
