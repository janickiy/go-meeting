package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// PresenceTimeout ограничивает время отсутствия подтверждённой связи: после пяти
// секунд без pong физическое соединение больше не считается действующим.
const PresenceTimeout = 5 * time.Second

// RealtimeConfig задаёт лимиты, сроки жизни, допустимые источники и параметры WebSocket-подключений.
//   - PingInterval: значение PingInterval типа time.Duration, используемое согласно назначению этой операции.
//   - PongTimeout: значение PongTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - WriteTimeout: значение WriteTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - SessionTTL: значение SessionTTL типа time.Duration, используемое согласно назначению этой операции.
//   - TicketTTL: значение TicketTTL типа time.Duration, используемое согласно назначению этой операции.
//   - QueueSize: значение QueueSize типа int, используемое согласно назначению этой операции.
//   - MessageBytes: значение MessageBytes типа int64, используемое согласно назначению этой операции.
//   - OutboundBytes: значение OutboundBytes типа int, используемое согласно назначению этой операции.
//   - SDPBytes: значение SDPBytes типа int, используемое согласно назначению этой операции.
//   - ICEBytes: значение ICEBytes типа int, используемое согласно назначению этой операции.
//   - MessagesPerSecond: значение MessagesPerSecond типа int, используемое согласно назначению этой операции.
//   - Burst: значение Burst типа int, используемое согласно назначению этой операции.
//   - Namespace: пространство изолированных ключей и каналов Redis.
//   - AllowedOrigins: набор значений AllowedOrigins для последовательной или пакетной обработки.
//   - ICE: значение ICE типа realtime.ICEConfig, используемое согласно назначению этой операции.
type RealtimeConfig struct {
	PingInterval      time.Duration
	PongTimeout       time.Duration
	WriteTimeout      time.Duration
	SessionTTL        time.Duration
	TicketTTL         time.Duration
	QueueSize         int
	MessageBytes      int64
	OutboundBytes     int
	SDPBytes          int
	ICEBytes          int
	MessagesPerSecond int
	Burst             int
	Namespace         string
	AllowedOrigins    []string
	ICE               realtime.ICEConfig
	TURN              TURNConfig
	MaxConnections    int
}

// LoadRealtime читает и проверяет ограничения WebSocket, присутствия, сроков и очередей.
//
// @return:
//   - результат 1 (RealtimeConfig): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func LoadRealtime() (RealtimeConfig, error) {
	c := RealtimeConfig{
		PingInterval:      envDuration("WS_PING_INTERVAL", time.Second),
		PongTimeout:       envDuration("WS_PONG_TIMEOUT", 4*time.Second),
		WriteTimeout:      envDuration("WS_WRITE_TIMEOUT", 5*time.Second),
		SessionTTL:        envDuration("WS_SESSION_TTL", PresenceTimeout),
		TicketTTL:         envDuration("WS_TICKET_TTL", 45*time.Second),
		QueueSize:         envInt("WS_QUEUE_SIZE", 64),
		MessageBytes:      int64(envInt("WS_MAX_MESSAGE_BYTES", 65536)),
		OutboundBytes:     envInt("WS_MAX_OUTBOUND_BYTES", 262144),
		SDPBytes:          envInt("WS_MAX_SDP_BYTES", 49152),
		ICEBytes:          envInt("WS_MAX_ICE_BYTES", 4096),
		MessagesPerSecond: envInt("WS_MESSAGES_PER_SECOND", 20),
		Burst:             envInt("WS_MESSAGE_BURST", 40),
		MaxConnections:    envInt("WS_MAX_CONNECTIONS", 1000),
		Namespace:         env("WS_REDIS_NAMESPACE", "go-recorder:realtime:v1"),
		AllowedOrigins:    envList("WS_ALLOWED_ORIGINS"),
		ICE:               realtime.ICEConfig{ICEServers: []realtime.ICEServer{}},
	}
	if raw := strings.TrimSpace(os.Getenv("WEBRTC_ICE_SERVERS_JSON")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.ICE.ICEServers); err != nil {
			return c, fmt.Errorf("WEBRTC_ICE_SERVERS_JSON must be an ICE server array")
		}
	}
	var err error
	c.TURN, err = LoadTURN()
	if err != nil {
		return c, err
	}
	return c, c.Validate()
}

// Validate проверяет ограничения и согласованность полей текущего значения перед его использованием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c RealtimeConfig) Validate() error {
	if c.MaxConnections < 0 || c.MaxConnections > 100000 {
		return fmt.Errorf("invalid WS_MAX_CONNECTIONS")
	}
	if c.PingInterval < 100*time.Millisecond || c.PingInterval > time.Minute ||
		c.PongTimeout < 10*time.Millisecond || c.PongTimeout > 30*time.Second ||
		c.WriteTimeout < 10*time.Millisecond || c.WriteTimeout > 30*time.Second ||
		c.PingInterval+c.PongTimeout > PresenceTimeout ||
		c.SessionTTL < c.PingInterval+c.PongTimeout || c.SessionTTL > PresenceTimeout ||
		c.TicketTTL < 30*time.Second || c.TicketTTL > 60*time.Second ||
		c.QueueSize < 1 || c.QueueSize > 4096 || c.MessageBytes < 1024 || c.MessageBytes > 1048576 ||
		int64(c.OutboundBytes) < c.MessageBytes+1024 || c.OutboundBytes > 1048576 ||
		c.SDPBytes < 1 || int64(c.SDPBytes) > c.MessageBytes || c.ICEBytes < 1 || int64(c.ICEBytes) > c.MessageBytes ||
		c.MessagesPerSecond < 1 || c.MessagesPerSecond > 10000 || c.Burst < 1 || c.Burst > 10000 || len(c.Namespace) == 0 || len(c.Namespace) > 128 {
		return fmt.Errorf("invalid WebSocket limits/intervals; ticket TTL must be 30–60s, ping+pong <= 5s and ping+pong <= session TTL <= 5s")
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("WS_ALLOWED_ORIGINS must contain only HTTP(S) origins")
		}
	}
	for _, server := range c.ICE.ICEServers {
		if len(server.URLs) == 0 {
			return fmt.Errorf("ICE server urls cannot be empty")
		}
		for _, address := range server.URLs {
			u, err := url.Parse(address)
			if err != nil || !strings.Contains("|stun|stuns|turn|turns|", "|"+u.Scheme+"|") || u.Opaque == "" || strings.ContainsAny(address, "\r\n ") {
				return fmt.Errorf("invalid ICE server URL")
			}
		}
	}
	return nil
}
