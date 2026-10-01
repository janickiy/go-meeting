package config

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"strings"
	"testing"
	"time"
)

// TestTURNTemporaryCredentials проверяет REST HMAC, срок, случайный username и
// отсутствие shared secret в браузерном JSON. t получает ошибки контракта.
func TestTURNTemporaryCredentials(t *testing.T) {
	c := TURNConfig{URLs: []string{"turn:relay.example:3478?transport=tcp"}, Secret: strings.Repeat("secret", 8), TTL: 5 * time.Minute, ForceRelay: true}
	now := time.Unix(1000000, 0)
	base := realtime.ICEConfig{}
	a, b := c.ClientICE(base, now), c.ClientICE(base, now)
	server := a.ICEServers[0]
	mac := hmac.New(sha1.New, []byte(c.Secret))
	_, _ = mac.Write([]byte(server.Username))
	if server.Credential != base64.StdEncoding.EncodeToString(mac.Sum(nil)) || a.ExpiresAt != now.Add(c.TTL).Unix() || !strings.HasPrefix(server.Username, fmt.Sprintf("%d:", a.ExpiresAt)) || server.Username == b.ICEServers[0].Username || a.ICETransportPolicy != "relay" {
		t.Fatal("invalid REST credential")
	}
	data, _ := json.Marshal(a)
	if strings.Contains(string(data), c.Secret) || len(base.ICEServers) != 0 {
		t.Fatal("secret/config leaked")
	}
}

// TestTURNRejectsMalformedConfiguration проверяет fail-fast URL/TTL/ключа.
// t изолирует env каждого варианта, не читая настоящий .env проекта.
func TestTURNRejectsMalformedConfiguration(t *testing.T) {
	for _, url := range []string{"http://relay", "turn:user:pass@relay", "turn:relay:99999", "turn:relay?transport=bad", "turn:relay?transport=tcp&x=y"} {
		t.Run(url, func(t *testing.T) {
			t.Setenv("TURN_URLS", url)
			t.Setenv("TURN_SHARED_SECRET", strings.Repeat("t", 32))
			t.Setenv("TURN_CREDENTIAL_TTL", "10m")
			if _, err := LoadTURN(); err == nil {
				t.Fatal("malformed TURN accepted")
			}
		})
	}
	t.Setenv("TURN_URLS", "")
	t.Setenv("TURN_FORCE_RELAY", "true")
	if _, err := LoadTURN(); err == nil {
		t.Fatal("relay without TURN")
	}
}

// TestProductionRejectsUnsafeDefaults проверяет независимые secrets, HTTPS и
// отсутствие dev-паролей. t устанавливает только фиктивные тестовые значения.
func TestProductionRejectsUnsafeDefaults(t *testing.T) {
	for i, key := range []string{"JWT_SECRET", "MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "WORKER_INTERNAL_SECRET", "METRICS_SECRET", "TURN_SHARED_SECRET"} {
		t.Setenv(key, strings.Repeat(fmt.Sprint(i), 40))
	}
	t.Setenv("TURN_URLS", "turn:relay.example:3478")
	for _, key := range []string{"POSTGRES_USER", "POSTGRES_DB", "MINIO_ROOT_USER", "RABBIT_MQ_USER", "MEDIA_NAT_IPS"} {
		t.Setenv(key, "production-test")
	}
	for _, key := range []string{"POSTGRES_PASSWORD", "RABBIT_MQ_PASSWORD", "MINIO_ROOT_PASSWORD", "REDIS_PASSWORD"} {
		t.Setenv(key, strings.Repeat("random-test", 4))
	}
	t.Setenv("GIN_MODE", "release")
	t.Setenv("WS_ALLOWED_ORIGINS", "https://meet.example")
	cfg := Config{AppEnv: "production", RateLimitEnabled: true, MinIOPublicOrigin: "https://meet.example"}
	if err := validateProduction(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WS_ALLOWED_ORIGINS", "")
	if validateProduction(cfg) == nil {
		t.Fatal("empty production origin whitelist")
	}
	t.Setenv("WS_ALLOWED_ORIGINS", "https://meet.example")
	t.Setenv("POSTGRES_PASSWORD", "go_recorder_pass")
	if validateProduction(cfg) == nil {
		t.Fatal("default production password")
	}
	t.Setenv("POSTGRES_PASSWORD", strings.Repeat("test", 8))
	t.Setenv("WORKER_INTERNAL_SECRET", strings.Repeat("0", 40))
	if validateProduction(cfg) == nil {
		t.Fatal("shared JWT/internal key")
	}
}
