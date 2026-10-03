package config

import (
	"testing"
	"time"
)

// TestRealtimeConfigValidation проверяет сценарий «события реального времени конфигурация проверка входных данных», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRealtimeConfigValidation(t *testing.T) {
	c, err := LoadRealtime()
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RealtimeConfig){ /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*RealtimeConfig): значение настроек или состояния компонента согласно указанному типу.
		*/func(c *RealtimeConfig) { c.QueueSize = 0 }, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*RealtimeConfig): значение настроек или состояния компонента согласно указанному типу.
		*/func(c *RealtimeConfig) { c.TicketTTL = time.Minute + time.Second }, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*RealtimeConfig): значение настроек или состояния компонента согласно указанному типу.
		*/func(c *RealtimeConfig) { c.SessionTTL = time.Second }, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*RealtimeConfig): значение настроек или состояния компонента согласно указанному типу.
		*/func(c *RealtimeConfig) { c.MessageBytes = 0 }, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*RealtimeConfig): значение настроек или состояния компонента согласно указанному типу.
		*/func(c *RealtimeConfig) { c.AllowedOrigins = []string{"https://example.com/path"} }} {
		invalid := c
		change(&invalid)
		if invalid.Validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", `[{"urls":["turn:turn.example.com:3478"],"username":"test","credential":"not-production"}]`)
	if c, err = LoadRealtime(); err != nil || len(c.ICE.ICEServers) != 1 {
		t.Fatal("configured TURN rejected")
	}
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", `[{"urls":["https://example.com"]}]`)
	if _, err = LoadRealtime(); err == nil {
		t.Fatal("non-ICE URL accepted")
	}
}

// TestRealtimePresenceTimeout проверяет пятисекундный предел, согласованность
// сроков присутствия и отказ запуска со старой медленной проверкой связи.
//
// @args
//   - t: контекст проверки конфигурации без сетевых зависимостей.
func TestRealtimePresenceTimeout(t *testing.T) {
	for _, name := range []string{"WS_PING_INTERVAL", "WS_PONG_TIMEOUT", "WS_SESSION_TTL"} {
		t.Setenv(name, "")
	}
	c, err := LoadRealtime()
	if err != nil {
		t.Fatal(err)
	}
	if c.PingInterval != time.Second || c.PongTimeout != 4*time.Second || c.SessionTTL != PresenceTimeout {
		t.Fatalf("unexpected presence defaults: ping=%s pong=%s ttl=%s", c.PingInterval, c.PongTimeout, c.SessionTTL)
	}
	for _, change := range []struct {
		name string
		edit func(*RealtimeConfig)
	}{
		{"old ping", func(c *RealtimeConfig) { c.PingInterval = 25 * time.Second }},
		{"old pong", func(c *RealtimeConfig) { c.PongTimeout = 10 * time.Second }},
		{"old lease", func(c *RealtimeConfig) { c.SessionTTL = 75 * time.Second }},
		{"deadline above five seconds", func(c *RealtimeConfig) { c.PongTimeout += time.Millisecond }},
		{"lease before socket deadline", func(c *RealtimeConfig) { c.SessionTTL -= time.Millisecond }},
	} {
		t.Run(change.name, func(t *testing.T) {
			invalid := c
			change.edit(&invalid)
			if invalid.Validate() == nil {
				t.Fatal("configuration may retain stale presence")
			}
		})
	}
	c.PongTimeout = time.Second
	c.SessionTTL = 2 * time.Second
	if err := c.Validate(); err != nil {
		t.Fatalf("faster valid heartbeat rejected: %v", err)
	}
	t.Setenv("WS_PING_INTERVAL", "25s")
	if _, err := LoadRealtime(); err == nil {
		t.Fatal("old deployment environment accepted")
	}
}
