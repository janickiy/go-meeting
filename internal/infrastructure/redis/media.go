package redis

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/media"
	goredis "github.com/redis/go-redis/v9"
)

// Worker liveness and owner claims share Redis time and atomic operations. A
// lease ID fences an expired owner from renewing or deleting a new assignment.
var mediaRegistryScript = goredis.NewScript(`
local prefix,op,id = ARGV[1],ARGV[2],ARGV[3]
local index=prefix..':workers'
local clock=redis.call('TIME')
local now=tonumber(clock[1])*1000+math.floor(tonumber(clock[2])/1000)
local function worker(wid)
    local raw=redis.call('GET',prefix..':worker:'..wid)
    if not raw then return nil end
    return cjson.decode(raw)
end
local function matches(a,b)
    return a.workerId==b.workerId and a.leaseId==b.leaseId and a.endpoint==b.endpoint
end
if op=='register' then
    local incoming=cjson.decode(ARGV[4])
    local current=worker(id)
    if current and current.endpoint~=incoming.endpoint then return 0 end
    local ttl=tonumber(ARGV[5])
    redis.call('SET',prefix..':worker:'..id,ARGV[4],'PX',ttl)
    redis.call('ZADD',index,now+ttl,id)
    return 1
elseif op=='workers' then
    for _,wid in ipairs(redis.call('ZRANGEBYSCORE',index,'-inf',now)) do
        redis.call('ZREM',index,wid)
    end
    local result={}
    for _,wid in ipairs(redis.call('ZRANGEBYSCORE',index,now+1,'+inf')) do
        local raw=redis.call('GET',prefix..':worker:'..wid)
        if raw then table.insert(result,raw) else redis.call('ZREM',index,wid) end
    end
    return result
elseif op=='remove' then
    local w=worker(id)
    if w and w.endpoint==ARGV[4] then
        redis.call('DEL',prefix..':worker:'..id)
        redis.call('ZREM',index,id)
    end
    return 1
end
local key=prefix..':owner:'..id
local raw=redis.call('GET',key)
local current=nil
if raw then
    current=cjson.decode(raw)
    local w=worker(current.workerId)
    if not w or w.endpoint~=current.endpoint then
        redis.call('DEL',key)
        current=nil
    end
end
if op=='get' then return current and raw or '' end
local wanted=cjson.decode(ARGV[4])
if op=='claim' then
    if current then return raw end
    local w=worker(wanted.workerId)
    if not w then return '' end
    wanted.endpoint=w.endpoint
    raw=cjson.encode(wanted)
    redis.call('SET',key,raw,'PX',tonumber(ARGV[5]))
    return raw
elseif op=='renew' then
    if not current or not matches(current,wanted) then return 0 end
    redis.call('PEXPIRE',key,tonumber(ARGV[5]))
    return 1
elseif op=='release' then
    if current and matches(current,wanted) then redis.call('DEL',key) end
    return 1
end
return 0
`)

type MediaRegistry struct {
	client *goredis.Client
	prefix string
}

func NewMediaRegistry(client *goredis.Client, prefix string) *MediaRegistry {
	return &MediaRegistry{client: client, prefix: prefix}
}

func (s *MediaRegistry) action(ctx context.Context, op, id, raw string, ttl time.Duration) *goredis.Cmd {
	return mediaRegistryScript.Run(ctx, s.client, []string{s.prefix + ":workers"}, s.prefix, op, id, raw, ttl.Milliseconds())
}

func validWorkerID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, ":/\r\n \t")
}

func (s *MediaRegistry) RegisterWorker(ctx context.Context, worker media.Worker, ttl time.Duration) error {
	if !validWorkerID(worker.ID) || config.ValidateMediaEndpoint(worker.Endpoint) != nil || ttl < time.Millisecond {
		return media.ErrInvalid
	}
	raw, _ := json.Marshal(worker)
	n, err := s.action(ctx, "register", worker.ID, string(raw), ttl).Int()
	if err == nil && n != 1 {
		return media.ErrOwnership
	}
	return err
}

func (s *MediaRegistry) Workers(ctx context.Context) ([]media.Worker, error) {
	raws, err := s.action(ctx, "workers", "", "", 0).StringSlice()
	if err != nil {
		return nil, err
	}
	workers := make([]media.Worker, 0, len(raws))
	for _, raw := range raws {
		var w media.Worker
		if json.Unmarshal([]byte(raw), &w) != nil || !validWorkerID(w.ID) || config.ValidateMediaEndpoint(w.Endpoint) != nil {
			return nil, media.ErrUnavailable
		}
		workers = append(workers, w)
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].ID < workers[j].ID })
	return workers, nil
}

func (s *MediaRegistry) Claim(ctx context.Context, conferenceID, workerID string, ttl time.Duration) (media.Route, error) {
	if _, err := uuid.Parse(conferenceID); err != nil || !validWorkerID(workerID) || ttl < time.Millisecond {
		return media.Route{}, media.ErrInvalid
	}
	wanted := media.Route{WorkerID: workerID, LeaseID: uuid.NewString()}
	raw, _ := json.Marshal(wanted)
	value, err := s.action(ctx, "claim", conferenceID, string(raw), ttl).Text()
	if err != nil {
		return media.Route{}, err
	}
	if value == "" {
		return media.Route{}, media.ErrUnavailable
	}
	return decodeMediaRoute(value)
}

func decodeMediaRoute(raw string) (media.Route, error) {
	var route media.Route
	if json.Unmarshal([]byte(raw), &route) != nil {
		return route, media.ErrOwnership
	}
	lease, err := uuid.Parse(route.LeaseID)
	if err != nil || lease == uuid.Nil || !validWorkerID(route.WorkerID) || config.ValidateMediaEndpoint(route.Endpoint) != nil {
		return media.Route{}, media.ErrOwnership
	}
	return route, nil
}

func (s *MediaRegistry) GetOwner(ctx context.Context, conferenceID string) (media.Route, error) {
	value, err := s.action(ctx, "get", conferenceID, "", 0).Text()
	if errors.Is(err, goredis.Nil) || (err == nil && value == "") {
		return media.Route{}, media.ErrOwnership
	}
	if err != nil {
		return media.Route{}, err
	}
	return decodeMediaRoute(value)
}

func (s *MediaRegistry) Renew(ctx context.Context, conferenceID string, route media.Route, ttl time.Duration) error {
	if ttl < time.Millisecond {
		return media.ErrInvalid
	}
	raw, _ := json.Marshal(route)
	n, err := s.action(ctx, "renew", conferenceID, string(raw), ttl).Int()
	if err == nil && n != 1 {
		return media.ErrOwnership
	}
	return err
}

func (s *MediaRegistry) Release(ctx context.Context, conferenceID string, route media.Route) error {
	raw, _ := json.Marshal(route)
	return s.action(ctx, "release", conferenceID, string(raw), 0).Err()
}

func (s *MediaRegistry) RemoveWorker(ctx context.Context, workerID, endpoint string) error {
	return s.action(ctx, "remove", workerID, endpoint, 0).Err()
}
