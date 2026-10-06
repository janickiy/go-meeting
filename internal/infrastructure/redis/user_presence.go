package redis

import (
	"context"
	goredis "github.com/redis/go-redis/v9"
	"time"
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
