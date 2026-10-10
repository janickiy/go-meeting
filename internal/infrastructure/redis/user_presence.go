package redis

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	goredis "github.com/redis/go-redis/v9"
)

// One lease per physical account socket. Pruning and mutation are atomic across APIs.
type UserPresence struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

func NewUserPresence(c *goredis.Client, namespace string, ttl time.Duration) *UserPresence {
	return &UserPresence{c, namespace + ":user-presence:", ttl}
}

var userPresenceScript = goredis.NewScript(`
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
if ARGV[2]=='remove' then redis.call('ZREM',KEYS[1],ARGV[1]) else redis.call('ZADD',KEYS[1],now+tonumber(ARGV[3]),ARGV[1]) end
local n=redis.call('ZCARD',KEYS[1]);if n==0 then redis.call('DEL',KEYS[1]) else redis.call('PEXPIRE',KEYS[1],ARGV[3]) end
return n`)

var userPresenceCountScript = goredis.NewScript(`
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
return redis.call('ZCOUNT',KEYS[1],'('..now,'+inf')`)

func (p *UserPresence) Touch(ctx context.Context, user, connection string) (int64, error) {
	return p.change(ctx, user, connection, "touch")
}
func (p *UserPresence) Remove(ctx context.Context, user, connection string) (int64, error) {
	return p.change(ctx, user, connection, "remove")
}
func (p *UserPresence) change(ctx context.Context, user, id, action string) (int64, error) {
	return userPresenceScript.Run(ctx, p.client, []string{p.prefix + user}, id, action, p.ttl.Milliseconds()).Int64()
}
func (p *UserPresence) Count(ctx context.Context, user string) (int64, error) {
	return userPresenceCountScript.Run(ctx, p.client, []string{p.prefix + user}).Int64()
}

// A page uses one Redis command and a shared Redis clock, independent of API clocks.
const userPresenceOnlineScript = `
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
local result={}
for i,key in ipairs(KEYS) do result[i]=redis.call('ZCOUNT',key,'('..now,'+inf') end
return result`

// Online projects confirmed global account leases without extending their lifetime.
func (p *UserPresence) Online(ctx context.Context, users []string) (map[string]bool, error) {
	if len(users) > 100 {
		return nil, apperrors.ErrInvalidInput
	}
	keys, ids := []string{}, []string{}
	seen := make(map[string]bool, len(users))
	for _, user := range users {
		if user == "" || seen[user] {
			continue
		}
		seen[user] = true
		keys, ids = append(keys, p.prefix+user), append(ids, user)
	}
	result := make(map[string]bool, len(ids))
	if len(keys) == 0 {
		return result, nil
	}
	// EVAL avoids the extra NOSCRIPT retry on an API instance's first lookup.
	counts, err := p.client.Eval(ctx, userPresenceOnlineScript, keys).Int64Slice()
	if err != nil {
		return nil, err
	}
	if len(counts) != len(ids) {
		return nil, apperrors.ErrUnavailable
	}
	for i, id := range ids {
		result[id] = counts[i] > 0
	}
	return result, nil
}
