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

// Limiter задаёт контракт зависимого компонента Limiter в проверке HTTP-авторизации и ограничений запросов; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Allow: операция Allow с контрактом, описанным у метода.
type Limiter interface {
	// Allow проверяет ограничение частоты и возвращает решение, остаток и время сброса.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//   - limit (int): предел количества обрабатываемых элементов.
	//   - window (time.Duration): значение window типа time.Duration, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (ratelimit.Result): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error)
}

// KeyFunc строит идентификатор клиента для конкретного правила.
type KeyFunc func(c *gin.Context) string

// Rule описывает отдельное правило лимита, его область, ключ и временное окно.
// @params
//   - Method: значение Method типа string, используемое согласно назначению этой операции.
//   - Path: путь к локальному файлу или каталогу операции.
//   - Scope: область собственных и участвующих встреч пользователя.
//   - Limit: предел количества обрабатываемых элементов.
//   - Window: значение Window типа time.Duration, используемое согласно назначению этой операции.
//   - Key: ключ ограничителя, блокировки или объекта в соответствующем хранилище.
type Rule struct {
	Method string
	Path   string
	Scope  string
	Limit  int
	Window time.Duration
	Key    KeyFunc
}

// RateLimitConfig задаёт правила и режим обработки ограничения частоты запросов.
// @params
//   - Enabled: логический признак Enabled, управляющий соответствующей веткой обработки.
//   - Rules: набор значений Rules для последовательной или пакетной обработки.
type RateLimitConfig struct {
	Enabled bool
	Rules   []Rule
}

// RateLimit создаёт промежуточный обработчик Gin для проверки лимита запросов.
// @args
// - limiter: ограничитель запросов на основе Redis.
// - cfg: флаг включения и правила.
// @return Gin middleware.
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

	// Вложенный обработчик выполняет выделенный шаг обработки в проверке HTTP-авторизации и ограничений запросов, используя состояние окружающей функции.
	//
	// @args
	//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
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

// ClientIPKey возвращает IP клиента из контекста Gin.
// @args
// - c: контекст HTTP-запроса Gin.
// @return IP клиента.
func ClientIPKey(c *gin.Context) string {
	return c.ClientIP()
}

// PathParamKey возвращает значение параметра пути.
// @args
// - name: имя параметра route.
// @return функцию построения key.
func PathParamKey(name string) KeyFunc {
	// Вложенный обработчик выполняет выделенный шаг обработки в проверке HTTP-авторизации и ограничений запросов, используя состояние окружающей функции.
	//
	// @args
	//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
	return func(c *gin.Context) string {
		return c.Param(name)
	}
}

// JSONFieldKey возвращает поле тела JSON и восстанавливает тело для последующего обработчика.
// @args
// - field: имя JSON-поля.
// @return функцию построения key.
func JSONFieldKey(field string) KeyFunc {
	// Вложенный обработчик выполняет выделенный шаг обработки в проверке HTTP-авторизации и ограничений запросов, используя состояние окружающей функции.
	//
	// @args
	//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
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

// parsedJSONBody разбирает и кеширует JSON-тело для извлечения ключа лимита, сохраняя тело для следующего обработчика.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (map[string]any): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
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

// redisRateLimitKey строит изолированный ключ ограничения частоты из правила и идентичности.
//
// @args
//   - rule (Rule): значение rule типа Rule, используемое согласно назначению этой операции.
//   - identity (string): проверенная идентичность пользователя и его членства.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func redisRateLimitKey(rule Rule, identity string) string {
	return strings.Join([]string{
		"rate",
		safeKeyPart(rule.Scope),
		safeKeyPart(identity),
		safeKeyPart(rule.Method),
		safeKeyPart(rule.Path),
	}, ":")
}

// safeKeyPart нормализует компонент Redis-ключа, чтобы внешнее значение не меняло его структуру.
//
// @args
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func safeKeyPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", ".")

	return value
}

// abortRateLimitExceeded завершает запрос ответом 429 и выставляет сведения об ограничении и повторной попытке.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - result (ratelimit.Result): результат проверки или обработки, передаваемый следующему шагу.
func abortRateLimitExceeded(c *gin.Context, result ratelimit.Result) {
	setRateLimitHeaders(c, result)
	if result.RetryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
	}
	abortRateLimitError(c, http.StatusTooManyRequests, "rate limit exceeded")
}

// abortRateLimitError возвращает безопасную ошибку проверки ограничения частоты.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - status (int): состояние ресурса, ответа или фильтра выборки.
//   - message (string): сообщение чата или безопасный текст ответа согласно указанному типу.
func abortRateLimitError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status":  "failed",
		"message": message,
	})
}

// setRateLimitHeaders записывает заголовки остатка лимита и времени сброса.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - result (ratelimit.Result): результат проверки или обработки, передаваемый следующему шагу.
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
