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

// Redis TIME, not API clocks, controls presence leases. The TTL routing key is
// authoritative; a sorted expiry index retains enough metadata to announce a
// crash after that key expires. All removal/count changes are atomic.
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
    local s = cjson.decode(raw)
    if redis.call('HEXISTS', meta, id) == 1 then return 0 end
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, now+ttl, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, now+ttl, id)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PX', ttl)
    redis.call('PUBLISH', channel, cjson.encode({kind='connected', conferenceId=s.conferenceId, session=s}))
    return 1
elseif op == 'leave' then
    remove(id, 'disconnected')
    return 1
elseif op == 'touch' then
    local raw = redis.call('GET', prefix .. ':route:' .. id)
    if not raw then return 0 end
    local s = cjson.decode(raw)
    s.lastSeenAt = ARGV[4]
    raw = cjson.encode(s)
    local ttl = tonumber(ARGV[5])
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PX', ttl)
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, now+ttl, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, now+ttl, id)
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

type RealtimeStore struct {
	client *goredis.Client
	prefix string
}

func NewRealtimeStore(client *goredis.Client, prefix string) *RealtimeStore {
	return &RealtimeStore{client: client, prefix: prefix}
}
func (s *RealtimeStore) action(ctx context.Context, op, id, raw string, ttl time.Duration) *goredis.Cmd {
	return presenceScript.Run(ctx, s.client, []string{s.prefix + ":metadata", s.prefix + ":expiry"}, s.prefix, op, id, raw, ttl.Milliseconds())
}
func (s *RealtimeStore) Register(ctx context.Context, session realtime.Session, ttl time.Duration) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	n, err := s.action(ctx, "join", session.ConnectionID, string(raw), ttl).Int()
	if err == nil && n != 1 {
		return apperrors.ErrConflict
	}
	return err
}
func (s *RealtimeStore) Unregister(ctx context.Context, id string) error {
	return s.action(ctx, "leave", id, "", 0).Err()
}
func (s *RealtimeStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	n, err := s.action(ctx, "touch", id, time.Now().UTC().Format(time.RFC3339Nano), ttl).Int()
	if err == nil && n != 1 {
		return apperrors.ErrNotFound
	}
	return err
}
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
func (s *RealtimeStore) Active(ctx context.Context, conferenceID string) ([]realtime.Session, error) {
	raw, err := s.action(ctx, "active", conferenceID, "", 0).StringSlice()
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
func (s *RealtimeStore) Prune(ctx context.Context) error {
	return s.action(ctx, "prune", "", "", 0).Err()
}
func (s *RealtimeStore) Publish(ctx context.Context, bus realtime.Bus) error {
	raw, err := json.Marshal(bus)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, s.prefix+":bus", raw).Err()
}
func (s *RealtimeStore) Subscribe(ctx context.Context) (realtime.Subscription, error) {
	p := s.client.Subscribe(ctx, s.prefix+":bus")
	if _, err := p.Receive(ctx); err != nil {
		_ = p.Close()
		return nil, err
	}
	return &realtimeSubscription{p}, nil
}

type realtimeSubscription struct{ pub *goredis.PubSub }

func (s *realtimeSubscription) Close() error { return s.pub.Close() }
func (s *realtimeSubscription) Receive(ctx context.Context) (realtime.Bus, error) {
	m, err := s.pub.ReceiveMessage(ctx)
	if err != nil {
		return realtime.Bus{}, err
	}
	var bus realtime.Bus
	err = json.Unmarshal([]byte(m.Payload), &bus)
	return bus, err
}
func (s *RealtimeStore) ticketKey(ticket string) string {
	hash := sha256.Sum256([]byte(ticket))
	return s.prefix + ":ticket:" + hex.EncodeToString(hash[:])
}
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
