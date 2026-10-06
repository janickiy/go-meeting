package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// Граница аренды вычисляется от приёма pong сервером API и ограничивается
// Redis TIME+TTL; часы API и Redis должны быть синхронизированы. Ключ маршрута
// с абсолютным сроком истечения — источник истины. Индекс сохраняет метаданные
// для сообщения о сбое после исчезновения ключа. Все изменения атомарны.
var presenceScript = goredis.NewScript(`
local prefix = ARGV[1]
local op = ARGV[2]
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local meta = prefix .. ':metadata'
local expiry = prefix .. ':expiry'
local channel = prefix .. ':bus'
local function remove(id, kind)
    local raw = redis.call('HGET', meta, id)
    if raw then
        local s = cjson.decode(raw)
        redis.call('ZREM', prefix .. ':conference:' .. s.conferenceId, id)
        redis.call('PUBLISH', channel, cjson.encode({kind=kind, conferenceId=s.conferenceId, session=s}))
    end
    redis.call('DEL', prefix .. ':route:' .. id)
    redis.call('HDEL', meta, id)
    redis.call('ZREM', expiry, id)
end
for _, id in ipairs(redis.call('ZRANGEBYSCORE', expiry, '-inf', now, 'LIMIT', 0, 100)) do
    remove(id, 'expired')
end
local id = ARGV[3]
if op == 'join' then
    local raw = ARGV[4]
    local ttl = tonumber(ARGV[5])
    local deadline = math.min(tonumber(ARGV[6]), now+ttl)
    if deadline <= now then return 0 end
    local s = cjson.decode(raw)
    if redis.call('HEXISTS', meta, id) == 1 then return 0 end
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, deadline, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, deadline, id)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PXAT', deadline)
    redis.call('PUBLISH', channel, cjson.encode({kind='connected', conferenceId=s.conferenceId, session=s}))
    return 1
elseif op == 'leave' then
    remove(id, 'disconnected')
    return 1
elseif op == 'touch' then
    local raw = redis.call('GET', prefix .. ':route:' .. id)
    if not raw then return 0 end
    local ttl = tonumber(ARGV[5])
    local deadline = math.min(tonumber(ARGV[6]), now+ttl)
    if deadline <= now then return 0 end
    local s = cjson.decode(raw)
    s.lastSeenAt = ARGV[4]
    raw = cjson.encode(s)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PXAT', deadline)
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, deadline, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, deadline, id)
    return 1
elseif op == 'active' then
    local result = {}
    for _, cid in ipairs(redis.call('ZRANGEBYSCORE', prefix .. ':conference:' .. id, now+1, '+inf')) do
        local raw = redis.call('GET', prefix .. ':route:' .. cid)
        if raw then table.insert(result, raw) end
    end
    return result
end
return 1
`)

// RealtimeStore хранит распределённое присутствие, одноразовые билеты и доставляет события через Redis.
// @params
//   - client: клиент внешнего сервиса или транспорта компонента.
//   - prefix: ограниченный префикс объектов, относящихся к одной операции.
type RealtimeStore struct {
	client *goredis.Client
	prefix string
}

// NewRealtimeStore создаёт и связывает зависимости компонента RealtimeStore, используемого в присутствии участников и доставке realtime-событий.
//
// @args
//   - client (*goredis.Client): клиент внешнего сервиса или транспорта компонента.
//   - prefix (string): ограниченный префикс объектов, относящихся к одной операции.
//
// @return:
//   - результат 1 (*RealtimeStore): созданный компонент с переданными зависимостями.
func NewRealtimeStore(client *goredis.Client, prefix string) *RealtimeStore {
	return &RealtimeStore{client: client, prefix: prefix}
}

// action вызывает соответствующий Lua-сценарий Redis для атомарной работы с распределённым состоянием.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - op (string): значение op типа string, используемое согласно назначению этой операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//   - ttl (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
//   - deadline (time.Time): абсолютная граница аренды; не используется операциями без продления.
//
// @return:
//   - результат 1 (*goredis.Cmd): значение, подготовленное операцией для вызывающей стороны.
func (s *RealtimeStore) action(ctx context.Context, op, id, raw string, ttl time.Duration, deadline time.Time) *goredis.Cmd {
	return presenceScript.Run(ctx, s.client, []string{s.prefix + ":metadata", s.prefix + ":expiry"}, s.prefix, op, id, raw, ttl.Milliseconds(), deadline.UnixMilli())
}

// Register регистрирует физическое соединение и его ограниченное по времени присутствие.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (realtime.Session): историческая физическая сессия или состояние текущего соединения.
//   - ttl (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Register(ctx context.Context, session realtime.Session, ttl time.Duration) error {
	if session.LastSeenAt.IsZero() || ttl < time.Millisecond {
		return apperrors.ErrConflict
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	n, err := s.action(ctx, "join", session.ConnectionID, string(raw), ttl, session.LastSeenAt.Add(ttl)).Int()
	if err == nil && n != 1 {
		return apperrors.ErrConflict
	}
	return err
}

// Unregister закрывает физическую сессию и обновляет распределённое присутствие участника.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Unregister(ctx context.Context, id string) error {
	return s.action(ctx, "leave", id, "", 0, time.Time{}).Err()
}

// Touch продлевает присутствие до подтверждённого pong плюс ttl. Абсолютный
// дедлайн исключает продление на задержку сети или обработки в Redis; старое
// подтверждение не может воскресить уже истёкшее соединение. Время API и Redis
// должно быть синхронизировано; при сдвиге часов срок не превышает now Redis+ttl.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - confirmedAt (time.Time): момент получения подтверждённого pong сервером.
//   - ttl (time.Duration): срок присутствия после подтверждённого pong.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Touch(ctx context.Context, id string, confirmedAt time.Time, ttl time.Duration) error {
	if confirmedAt.IsZero() || ttl < time.Millisecond {
		return apperrors.ErrNotFound
	}
	n, err := s.action(ctx, "touch", id, confirmedAt.UTC().Format(time.RFC3339Nano), ttl, confirmedAt.Add(ttl)).Int()
	if err == nil && n != 1 {
		return apperrors.ErrNotFound
	}
	return err
}

// Get читает состояние физических сессий и событий комнаты для дальнейшей обработки или ответа.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (realtime.Session): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Get(ctx context.Context, id string) (realtime.Session, error) {
	raw, err := s.client.Get(ctx, s.prefix+":route:"+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return realtime.Session{}, apperrors.ErrNotFound
	}
	var session realtime.Session
	if err == nil {
		err = json.Unmarshal(raw, &session)
	}
	return session, err
}

// Missing checks route existence without reading session JSON. Each pipeline is
// bounded to the reconciliation page size. Partial replies on transport errors
// are discarded so a Redis failure cannot close live SQL sessions.
func (s *RealtimeStore) Missing(ctx context.Context, ids []string) ([]string, error) {
	var missing []string
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		commands := make([]*goredis.IntCmd, len(batch))
		_, err := s.client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
			for i, id := range batch {
				commands[i] = pipe.Exists(ctx, s.prefix+":route:"+id)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for i, command := range commands {
			if command.Val() == 0 {
				missing = append(missing, batch[i])
			}
		}
	}
	return missing, nil
}

// Active возвращает действующие сессии, учитывая срок их активности.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 ([]realtime.Session): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Active(ctx context.Context, conferenceID string) ([]realtime.Session, error) {
	raw, err := s.action(ctx, "active", conferenceID, "", 0, time.Time{}).StringSlice()
	if err != nil {
		return nil, err
	}
	result := make([]realtime.Session, 0, len(raw))
	for _, entry := range raw {
		var session realtime.Session
		if err := json.Unmarshal([]byte(entry), &session); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, nil
}

// Prune удаляет просроченные сессии и возвращает сведения для восстановления присутствия.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Prune(ctx context.Context) error {
	return s.action(ctx, "prune", "", "", 0, time.Time{}).Err()
}

// Publish сериализует доверенное событие и публикует его в изолированном Redis-канале.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - bus (realtime.Bus): транспорт публикации и подписки на доверенные события.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Publish(ctx context.Context, bus realtime.Bus) error {
	raw, err := json.Marshal(bus)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, s.prefix+":bus", raw).Err()
}

// Subscribe открывает ограниченную по времени подписку на изолированный канал событий.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (realtime.Subscription): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) Subscribe(ctx context.Context) (realtime.Subscription, error) {
	p := s.client.Subscribe(ctx, s.prefix+":bus")
	if _, err := p.Receive(ctx); err != nil {
		_ = p.Close()
		return nil, err
	}
	return &realtimeSubscription{p}, nil
}

// realtimeSubscription оборачивает Redis Pub/Sub-подписку и её ограниченный жизненный цикл.
//   - pub: значение pub типа *goredis.PubSub, используемое согласно назначению этой операции.
type realtimeSubscription struct{ pub *goredis.PubSub }

// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *realtimeSubscription) Close() error { return s.pub.Close() }

// Receive ожидает и разбирает следующее сообщение активной подписки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (realtime.Bus): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *realtimeSubscription) Receive(ctx context.Context) (realtime.Bus, error) {
	m, err := s.pub.ReceiveMessage(ctx)
	if err != nil {
		return realtime.Bus{}, err
	}
	var bus realtime.Bus
	err = json.Unmarshal([]byte(m.Payload), &bus)
	return bus, err
}

// ticketKey строит изолированный Redis-ключ одноразового билета подключения.
//
// @args
//   - ticket (string): одноразовый билет ограниченного подключения.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (s *RealtimeStore) ticketKey(ticket string) string {
	hash := sha256.Sum256([]byte(ticket))
	return s.prefix + ":ticket:" + hex.EncodeToString(hash[:])
}

// SaveTicket сохраняет одноразовый билет подключения с ограниченным сроком жизни.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - ticket (string): одноразовый билет ограниченного подключения.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - identity (realtime.Identity): проверенная идентичность пользователя и его членства.
//   - ttl (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) SaveTicket(ctx context.Context, ticket, conferenceID string, identity realtime.Identity, ttl time.Duration) error {
	raw, err := json.Marshal(struct {
		ConferenceID string            `json:"conferenceId"`
		Identity     realtime.Identity `json:"identity"`
	}{conferenceID, identity})
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.ticketKey(ticket), raw, ttl).Err()
}

// ConsumeTicket атомарно забирает одноразовый билет, исключая повторное использование.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - ticket (string): одноразовый билет ограниченного подключения.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (realtime.Identity): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *RealtimeStore) ConsumeTicket(ctx context.Context, ticket, conferenceID string) (realtime.Identity, error) {
	if len(ticket) != 43 {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	raw, err := s.client.GetDel(ctx, s.ticketKey(ticket)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return realtime.Identity{}, err
	}
	var value struct {
		ConferenceID string            `json:"conferenceId"`
		Identity     realtime.Identity `json:"identity"`
	}
	if json.Unmarshal(raw, &value) != nil || value.ConferenceID != conferenceID || !value.Identity.ExpiresAt.After(time.Now()) {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	return value.Identity, nil
}
