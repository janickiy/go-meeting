package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

// MediaConfig задаёт настройки медиа-воркера, SFU, внутренней авторизации и распределённого владения.
//   - HTTPPort: значение HTTPPort типа int, используемое согласно назначению этой операции.
//   - WorkerID: идентификатор воркера-владельца операции.
//   - WorkerInternalURL: значение WorkerInternalURL типа string, используемое согласно назначению этой операции.
//   - Namespace: пространство изолированных ключей и каналов Redis.
//   - TicketSecret: значение TicketSecret типа string, используемое согласно назначению этой операции.
//   - InternalSecret: значение InternalSecret типа string, используемое согласно назначению этой операции.
//   - TicketTTL: значение TicketTTL типа time.Duration, используемое согласно назначению этой операции.
//   - OperationTimeout: значение OperationTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - HeartbeatInterval: значение HeartbeatInterval типа time.Duration, используемое согласно назначению этой операции.
//   - WorkerTTL: значение WorkerTTL типа time.Duration, используемое согласно назначению этой операции.
//   - OwnershipTTL: значение OwnershipTTL типа time.Duration, используемое согласно назначению этой операции.
//   - SessionCheckInterval: значение SessionCheckInterval типа time.Duration, используемое согласно назначению этой операции.
//   - MaxPeers: значение MaxPeers типа int, используемое согласно назначению этой операции.
//   - MaxRooms: значение MaxRooms типа int, используемое согласно назначению этой операции.
//   - MaxPublishedTracks: значение MaxPublishedTracks типа int, используемое согласно назначению этой операции.
//   - MaxAudioTracks: значение MaxAudioTracks типа int, используемое согласно назначению этой операции.
//   - MaxVideoTracks: значение MaxVideoTracks типа int, используемое согласно назначению этой операции.
//   - MaxScreenSharers: значение MaxScreenSharers типа int, используемое согласно назначению этой операции.
//   - EgressQueueSize: значение EgressQueueSize типа int, используемое согласно назначению этой операции.
//   - VideoMaxWidth: значение VideoMaxWidth типа int, используемое согласно назначению этой операции.
//   - VideoMaxHeight: значение VideoMaxHeight типа int, используемое согласно назначению этой операции.
//   - VideoMaxFPS: значение VideoMaxFPS типа int, используемое согласно назначению этой операции.
//   - UDPPort: значение UDPPort типа int, используемое согласно назначению этой операции.
//   - UDPMinPort: значение UDPMinPort типа int, используемое согласно назначению этой операции.
//   - UDPMaxPort: значение UDPMaxPort типа int, используемое согласно назначению этой операции.
//   - TCPPort: значение TCPPort типа int, используемое согласно назначению этой операции.
//   - NATIPs: набор значений NATIPs для последовательной или пакетной обработки.
//   - ICE: значение ICE типа realtime.ICEConfig, используемое согласно назначению этой операции.
//   - ICEDisconnectedTimeout: значение ICEDisconnectedTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - ICEFailedTimeout: значение ICEFailedTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - ICEKeepaliveInterval: значение ICEKeepaliveInterval типа time.Duration, используемое согласно назначению этой операции.
//   - NegotiationTimeout: значение NegotiationTimeout типа time.Duration, используемое согласно назначению этой операции.
type MediaConfig struct {
	HTTPPort               int
	WorkerID               string
	WorkerInternalURL      string
	Namespace              string
	TicketSecret           string
	InternalSecret         string
	TicketTTL              time.Duration
	OperationTimeout       time.Duration
	HeartbeatInterval      time.Duration
	WorkerTTL              time.Duration
	OwnershipTTL           time.Duration
	SessionCheckInterval   time.Duration
	MaxPeers               int
	MaxRooms               int
	MaxPublishedTracks     int
	MaxAudioTracks         int
	MaxVideoTracks         int
	MaxScreenSharers       int
	EgressQueueSize        int
	VideoMaxWidth          int
	VideoMaxHeight         int
	VideoMaxFPS            int
	UDPPort                int
	UDPMinPort             int
	UDPMaxPort             int
	TCPPort                int
	NATIPs                 []string
	ICE                    realtime.ICEConfig
	ICEDisconnectedTimeout time.Duration
	ICEFailedTimeout       time.Duration
	ICEKeepaliveInterval   time.Duration
	NegotiationTimeout     time.Duration
	TURN                   TURNConfig
}

// LoadMedia читает и проверяет настройки медиа-воркера, его внутреннего транспорта и ICE.
//
// @return:
//   - результат 1 (MediaConfig): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func LoadMedia() (MediaConfig, error) {
	c := MediaConfig{
		HTTPPort:               envInt("MEDIA_HTTP_PORT", 8091),
		WorkerID:               env("MEDIA_WORKER_ID", "media-worker-local"),
		WorkerInternalURL:      env("MEDIA_WORKER_INTERNAL_URL", "http://media-worker:8091"),
		Namespace:              env("MEDIA_REDIS_NAMESPACE", "go-recorder:media:v1"),
		TicketSecret:           os.Getenv("MEDIA_TICKET_SECRET"),
		InternalSecret:         os.Getenv("MEDIA_INTERNAL_SECRET"),
		TicketTTL:              envDuration("MEDIA_TICKET_TTL", 45*time.Second),
		OperationTimeout:       envDuration("MEDIA_OPERATION_TIMEOUT", 5*time.Second),
		HeartbeatInterval:      envDuration("MEDIA_HEARTBEAT_INTERVAL", 5*time.Second),
		WorkerTTL:              envDuration("MEDIA_WORKER_TTL", 20*time.Second),
		OwnershipTTL:           envDuration("MEDIA_OWNERSHIP_TTL", 20*time.Second),
		SessionCheckInterval:   envDuration("MEDIA_SESSION_CHECK_INTERVAL", 2*time.Second),
		MaxPeers:               envInt("MEDIA_MAX_PEERS", 10),
		MaxRooms:               envInt("MEDIA_MAX_ROOMS", 100),
		MaxPublishedTracks:     envInt("MEDIA_MAX_PUBLISHED_TRACKS", 4),
		MaxAudioTracks:         envInt("MEDIA_MAX_AUDIO_TRACKS", 2),
		MaxVideoTracks:         envInt("MEDIA_MAX_VIDEO_TRACKS", 2),
		MaxScreenSharers:       envInt("MEDIA_MAX_SCREEN_SHARERS", 1),
		EgressQueueSize:        envInt("MEDIA_EGRESS_QUEUE_SIZE", 2048),
		VideoMaxWidth:          envInt("MEDIA_VIDEO_MAX_WIDTH", 1280),
		VideoMaxHeight:         envInt("MEDIA_VIDEO_MAX_HEIGHT", 720),
		VideoMaxFPS:            envInt("MEDIA_VIDEO_MAX_FPS", 30),
		UDPPort:                envInt("MEDIA_UDP_PORT", 50010),
		UDPMinPort:             envInt("MEDIA_UDP_MIN_PORT", 0),
		UDPMaxPort:             envInt("MEDIA_UDP_MAX_PORT", 0),
		TCPPort:                envInt("MEDIA_TCP_PORT", 0),
		NATIPs:                 envList("MEDIA_NAT_IPS"),
		ICE:                    realtime.ICEConfig{ICEServers: []realtime.ICEServer{}},
		ICEDisconnectedTimeout: envDuration("MEDIA_ICE_DISCONNECTED_TIMEOUT", 10*time.Second),
		ICEFailedTimeout:       envDuration("MEDIA_ICE_FAILED_TIMEOUT", 20*time.Second),
		ICEKeepaliveInterval:   envDuration("MEDIA_ICE_KEEPALIVE_INTERVAL", 2*time.Second),
		NegotiationTimeout:     envDuration("MEDIA_NEGOTIATION_TIMEOUT", 15*time.Second),
	}
	// Сохраняем совместимость с локальным Compose без повторного использования исходных ключей токенов доступа.
	// В рабочей среде нужны независимые секреты; производные ключи разрешены только локально.
	local := Config{AppEnv: env("APP_ENV", "local")}.IsLocal()
	base := os.Getenv("JWT_SECRET")
	if local && len(base) >= 32 {
		if c.TicketSecret == "" {
			c.TicketSecret = mediaDerivedSecret(base, "media-ticket-v1")
		}
		if c.InternalSecret == "" {
			c.InternalSecret = mediaDerivedSecret(base, "media-internal-v1")
		}
	}
	iceKey := "MEDIA_ICE_SERVERS_JSON"
	raw := strings.TrimSpace(os.Getenv(iceKey))
	if raw == "" {
		iceKey = "WEBRTC_ICE_SERVERS_JSON"
		raw = strings.TrimSpace(os.Getenv(iceKey))
	}
	if raw != "" {
		if len(raw) > 16384 || json.Unmarshal([]byte(raw), &c.ICE.ICEServers) != nil {
			return c, fmt.Errorf("%s must be a bounded ICE server array", iceKey)
		}
	}
	var err error
	c.TURN, err = LoadTURN()
	if err != nil {
		return c, err
	}
	return c, c.Validate()
}

// mediaDerivedSecret вычисляет отдельный секрет назначения из базового секрета, исключая совместное использование ключей разных протоколов.
//
// @args
//   - base (string): значение base типа string, используемое согласно назначению этой операции.
//   - purpose (string): значение purpose типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func mediaDerivedSecret(base, purpose string) string {
	h := hmac.New(sha256.New, []byte(base))
	_, _ = h.Write([]byte("go-recorder:" + purpose))
	return hex.EncodeToString(h.Sum(nil))
}

// Validate проверяет ограничения и согласованность полей текущего значения перед его использованием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c MediaConfig) Validate() error {
	if len(c.TicketSecret) < 32 || len(c.InternalSecret) < 32 || c.TicketSecret == c.InternalSecret {
		return fmt.Errorf("MEDIA_TICKET_SECRET and MEDIA_INTERNAL_SECRET must be distinct and contain at least 32 bytes")
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 || len(c.WorkerID) < 1 || len(c.WorkerID) > 128 || strings.ContainsAny(c.WorkerID, ":/\r\n \t") ||
		len(c.Namespace) < 1 || len(c.Namespace) > 128 || strings.ContainsAny(c.Namespace, "\r\n \t") {
		return fmt.Errorf("invalid media worker identity, namespace or HTTP port")
	}
	if err := ValidateMediaEndpoint(c.WorkerInternalURL); err != nil {
		return err
	}
	if c.TicketTTL < 30*time.Second || c.TicketTTL > 60*time.Second ||
		c.OperationTimeout < 100*time.Millisecond || c.OperationTimeout > 30*time.Second ||
		c.HeartbeatInterval < 100*time.Millisecond || c.HeartbeatInterval > 30*time.Second ||
		c.WorkerTTL < 3*c.HeartbeatInterval || c.WorkerTTL > 5*time.Minute ||
		c.OwnershipTTL < 3*c.HeartbeatInterval || c.OwnershipTTL > 5*time.Minute ||
		c.SessionCheckInterval < 100*time.Millisecond || c.SessionCheckInterval > 30*time.Second ||
		c.NegotiationTimeout < time.Second || c.NegotiationTimeout > time.Minute ||
		c.ICEDisconnectedTimeout < time.Second || c.ICEFailedTimeout < c.ICEDisconnectedTimeout || c.ICEFailedTimeout > time.Minute ||
		c.ICEKeepaliveInterval < 100*time.Millisecond || c.ICEKeepaliveInterval >= c.ICEDisconnectedTimeout {
		return fmt.Errorf("invalid media ticket TTL, heartbeat, ownership or ICE timeouts")
	}
	if c.MaxPeers < 2 || c.MaxPeers > 32 || c.MaxRooms < 1 || c.MaxRooms > 10000 ||
		c.MaxPublishedTracks < 1 || c.MaxPublishedTracks > 4 || c.MaxAudioTracks < 1 || c.MaxAudioTracks > 2 ||
		c.MaxVideoTracks < 1 || c.MaxVideoTracks > 2 || c.MaxScreenSharers < 1 || c.MaxScreenSharers > 4 || c.EgressQueueSize < 128 || c.EgressQueueSize > 8192 ||
		c.VideoMaxWidth < 160 || c.VideoMaxWidth > 1280 || c.VideoMaxHeight < 90 || c.VideoMaxHeight > 720 || c.VideoMaxFPS < 1 || c.VideoMaxFPS > 30 {
		return fmt.Errorf("invalid media room, track or video limits")
	}
	for _, port := range []int{c.UDPPort, c.UDPMinPort, c.UDPMaxPort, c.TCPPort} {
		if port < 0 || port > 65535 {
			return fmt.Errorf("invalid media ICE port")
		}
	}
	if (c.UDPMinPort == 0) != (c.UDPMaxPort == 0) || c.UDPMaxPort < c.UDPMinPort ||
		(c.UDPPort > 0 && c.UDPMinPort > 0) {
		return fmt.Errorf("configure either MEDIA_UDP_PORT mux or a complete MEDIA_UDP_MIN_PORT/MAX_PORT range")
	}
	for _, ip := range c.NATIPs {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("MEDIA_NAT_IPS must contain IP addresses")
		}
	}
	return validateMediaICE(c.ICE)
}

// ValidateMediaEndpoint проверяет допустимый внутренний адрес медиа-воркера.
//
// @args
//   - address (string): адрес целевого внутреннего сервиса или сети.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func ValidateMediaEndpoint(address string) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || len(address) > 512 {
		return fmt.Errorf("MEDIA_WORKER_INTERNAL_URL must be an HTTP(S) origin without credentials")
	}
	return nil
}

// validateMediaICE проверяет формат настроек ICE и ограничения передачи учётных данных.
//
// @args
//   - c (realtime.ICEConfig): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func validateMediaICE(c realtime.ICEConfig) error {
	if len(c.ICEServers) > 16 {
		return fmt.Errorf("too many ICE servers")
	}
	for _, server := range c.ICEServers {
		if len(server.URLs) < 1 || len(server.URLs) > 8 || len(server.Username) > 512 || len(server.Credential) > 512 {
			return fmt.Errorf("invalid ICE server limits")
		}
		for _, address := range server.URLs {
			u, err := url.Parse(address)
			if err != nil || !strings.Contains("|stun|stuns|turn|turns|", "|"+u.Scheme+"|") || u.Opaque == "" || len(address) > 512 || strings.ContainsAny(address, "\r\n ") {
				return fmt.Errorf("invalid ICE server URL")
			}
		}
	}
	return nil
}
