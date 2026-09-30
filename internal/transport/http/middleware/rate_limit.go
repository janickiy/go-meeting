package httpmiddleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
)

const parsedBodyKey = "rate_limit_json_body"

// Limiter проверяет request key в хранилище rate limit.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error)
}

// KeyFunc строит идентификатор клиента для конкретного правила.
type KeyFunc func(c *gin.Context) string

// Rule описывает один rate limit для HTTP route.
type Rule struct {
	Method string
	Path   string
	Scope  string
	Limit  int
	Window time.Duration
	Key    KeyFunc
}

// RateLimitConfig содержит настройки middleware.
type RateLimitConfig struct {
	Enabled bool
	Rules   []Rule
}

// RateLimit создает Gin middleware для проверки rate limit.
// Параметры:
// - limiter: Redis-backed limiter.
// - cfg: флаг включения и правила.
// Возвращает: Gin middleware.
func RateLimit(limiter Limiter, cfg RateLimitConfig) gin.HandlerFunc {
	rulesByRoute := make(map[string][]Rule)
	for _, rule := range cfg.Rules {
		if rule.Limit <= 0 || rule.Key == nil {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(rule.Method))
		path := strings.TrimSpace(rule.Path)
		if method == "" || path == "" {
			continue
		}
		rule.Method = method
		rule.Path = path
		rule.Scope = strings.TrimSpace(rule.Scope)
		if rule.Scope == "" {
			rule.Scope = "default"
		}
		if rule.Window <= 0 {
			rule.Window = time.Minute
		}
		routeKey := method + " " + path
		rulesByRoute[routeKey] = append(rulesByRoute[routeKey], rule)
	}

	return func(c *gin.Context) {
		if !cfg.Enabled || limiter == nil {
			c.Next()
			return
		}
		routeRules := rulesByRoute[c.Request.Method+" "+c.FullPath()]
		if len(routeRules) == 0 {
			c.Next()
			return
		}

		for _, rule := range routeRules {
			identity := strings.TrimSpace(rule.Key(c))
			if identity == "" {
				continue
			}
			result, err := limiter.Allow(c.Request.Context(), redisRateLimitKey(rule, identity), rule.Limit, rule.Window)
			if err != nil {
				abortRateLimitError(c, http.StatusInternalServerError, "rate limit unavailable")
				return
			}
			if !result.Allowed {
				abortRateLimitExceeded(c, result)
				return
			}
			setRateLimitHeaders(c, result)
		}

		c.Next()
	}
}

// ClientIPKey возвращает IP клиента из Gin context.
// Параметры:
// - c: Gin context HTTP-запроса.
// Возвращает: IP клиента.
func ClientIPKey(c *gin.Context) string {
	return c.ClientIP()
}

// PathParamKey возвращает значение path parameter.
// Параметры:
// - name: имя параметра route.
// Возвращает: функцию построения key.
func PathParamKey(name string) KeyFunc {
	return func(c *gin.Context) string {
		return c.Param(name)
	}
}

// JSONFieldKey возвращает значение поля из JSON body и восстанавливает body для handler-а.
// Параметры:
// - field: имя JSON-поля.
// Возвращает: функцию построения key.
func JSONFieldKey(field string) KeyFunc {
	return func(c *gin.Context) string {
		payload, ok := parsedJSONBody(c)
		if !ok {
			return ""
		}
		value, ok := payload[field]
		if !ok {
			return ""
		}

		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func parsedJSONBody(c *gin.Context) (map[string]any, bool) {
	if cached, ok := c.Get(parsedBodyKey); ok {
		payload, ok := cached.(map[string]any)
		return payload, ok
	}
	if c.Request == nil || c.Request.Body == nil {
		return nil, false
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(nil))
		c.Set(parsedBodyKey, map[string]any{})
		return nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(bytes.TrimSpace(body)) == 0 {
		c.Set(parsedBodyKey, map[string]any{})
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.Set(parsedBodyKey, map[string]any{})
		return nil, false
	}
	c.Set(parsedBodyKey, payload)

	return payload, true
}

func redisRateLimitKey(rule Rule, identity string) string {
	return strings.Join([]string{
		"rate",
		safeKeyPart(rule.Scope),
		safeKeyPart(identity),
		safeKeyPart(rule.Method),
		safeKeyPart(rule.Path),
	}, ":")
}

func safeKeyPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", ".")

	return value
}

func abortRateLimitExceeded(c *gin.Context, result ratelimit.Result) {
	setRateLimitHeaders(c, result)
	if result.RetryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
	}
	abortRateLimitError(c, http.StatusTooManyRequests, "rate limit exceeded")
}

func abortRateLimitError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status":  "failed",
		"message": message,
	})
}

func setRateLimitHeaders(c *gin.Context, result ratelimit.Result) {
	if result.Limit > 0 {
		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	}
	if result.Remaining >= 0 {
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	}
	if !result.ResetAt.IsZero() {
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}
}
