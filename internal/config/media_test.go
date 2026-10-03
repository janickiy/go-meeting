package config

import (
	"strings"
	"testing"
	"time"
)

// mediaTestEnvironment подготавливает или проверяет часть тестового сценария «медиа проверка Environment».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func mediaTestEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "WEBRTC_ICE_SERVERS_JSON", "MEDIA_ICE_SERVERS_JSON", "MEDIA_UDP_MIN_PORT", "MEDIA_UDP_MAX_PORT", "MEDIA_UDP_PORT", "MEDIA_NAT_IPS", "MEDIA_WORKER_INTERNAL_URL", "MEDIA_WORKER_ID", "MEDIA_MAX_PEERS", "MEDIA_MAX_PUBLISHED_TRACKS", "MEDIA_MAX_AUDIO_TRACKS", "MEDIA_MAX_VIDEO_TRACKS", "MEDIA_MAX_SCREEN_SHARERS", "MEDIA_EGRESS_QUEUE_SIZE", "MEDIA_TICKET_TTL", "MEDIA_HEARTBEAT_INTERVAL", "MEDIA_WORKER_TTL", "MEDIA_OWNERSHIP_TTL"} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "local")
	t.Setenv("JWT_SECRET", strings.Repeat("test-key-", 8))
}

// TestMediaConfigDefaultsAndLocalKeySeparation проверяет сценарий «медиа конфигурация Defaults и локальный ключ Separation», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaConfigDefaultsAndLocalKeySeparation(t *testing.T) {
	mediaTestEnvironment(t)
	c, err := LoadMedia()
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxPeers != 10 || c.MaxPublishedTracks != 4 || c.MaxAudioTracks != 2 || c.MaxVideoTracks != 2 || c.MaxScreenSharers != 1 || c.VideoMaxWidth != 1280 || c.VideoMaxHeight != 720 || c.VideoMaxFPS != 30 || c.UDPPort == 50000 {
		t.Fatalf("unexpected media defaults: peers=%d tracks=%d", c.MaxPeers, c.MaxPublishedTracks)
	}
	if c.TicketSecret == c.InternalSecret || c.TicketSecret == strings.Repeat("test-key-", 8) || len(c.TicketSecret) != 64 {
		t.Fatal("media keys must be domain-separated")
	}
	again, _ := LoadMedia()
	if again.TicketSecret != c.TicketSecret || again.InternalSecret != c.InternalSecret {
		t.Fatal("API and worker must derive the same local keys")
	}
}

// TestMediaProductionRequiresIndependentSecrets проверяет требование независимых секретов в рабочей среде.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaProductionRequiresIndependentSecrets(t *testing.T) {
	mediaTestEnvironment(t)
	t.Setenv("APP_ENV", "production")
	if _, err := LoadMedia(); err == nil {
		t.Fatal("production may not derive media secrets")
	}
	t.Setenv("MEDIA_TICKET_SECRET", strings.Repeat("t", 32))
	t.Setenv("MEDIA_INTERNAL_SECRET", strings.Repeat("i", 32))
	if _, err := LoadMedia(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIA_INTERNAL_SECRET", strings.Repeat("t", 32))
	if _, err := LoadMedia(); err == nil {
		t.Fatal("one shared key should be rejected")
	}
}

// TestMediaConfigRejectsUnsafeLimitsAndEndpoints проверяет отказ для небезопасных лимитов и адресов.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaConfigRejectsUnsafeLimitsAndEndpoints(t *testing.T) {
	mediaTestEnvironment(t)
	base, err := LoadMedia()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*MediaConfig)
	}{
		{"ticket ttl", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.TicketTTL = time.Minute + time.Second }},
		{"heartbeat lease", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.OwnershipTTL = c.HeartbeatInterval }},
		{"worker lease", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.WorkerTTL = c.HeartbeatInterval }},
		{"room flooding", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.MaxPeers = 100 }},
		{"track flooding", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.MaxPublishedTracks = 100 }},
		{"unsupported audio slots", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.MaxAudioTracks = 3 }},
		{"unsupported video slots", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.MaxVideoTracks = 3 }},
		{"oversized video", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.VideoMaxWidth = 1920 }},
		{"endpoint credentials", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.WorkerInternalURL = "http://user:secret@worker:8091" }},
		{"endpoint query", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.WorkerInternalURL = "http://worker:8091/?secret=x" }},
		{"partial UDP range", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.UDPPort = 0; c.UDPMinPort = 51000 }},
		{"mux and range", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.UDPMinPort = 51000; c.UDPMaxPort = 51020 }},
		{"invalid public address", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*MediaConfig): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *MediaConfig) { c.NATIPs = []string{"example.org"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

			@args
			  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
			*/func(t *testing.T) {
				c := base
				tc.change(&c)
				if c.Validate() == nil {
					t.Fatal("unsafe config accepted")
				}
			})
	}
}

// TestMediaICEFromEnvironment проверяет сценарий «медиа ICE из Environment», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaICEFromEnvironment(t *testing.T) {
	mediaTestEnvironment(t)
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", `[{"urls":["turn:turn.example.org:3478?transport=udp"],"username":"example","credential":"test-only"}]`)
	c, err := LoadMedia()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ICE.ICEServers) != 1 || c.ICE.ICEServers[0].Username != "example" {
		t.Fatal("TURN config not preserved")
	}
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", `[{"urls":["https://bad.example.org"]}]`)
	if _, err := LoadMedia(); err == nil {
		t.Fatal("non-ICE URL accepted")
	}
}

// TestMediaICEOverrideSeparatesSFUFromRecorderConfiguration проверяет раздельную настройку ICE для SFU и записи.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaICEOverrideSeparatesSFUFromRecorderConfiguration(t *testing.T) {
	mediaTestEnvironment(t)
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", `[{"urls":["stun:recorder.example.org:3478"]}]`)
	t.Setenv("MEDIA_ICE_SERVERS_JSON", `[{"urls":["turn:sfu.example.org:3478"],"username":"media","credential":"test-only"}]`)
	c, err := LoadMedia()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ICE.ICEServers) != 1 || c.ICE.ICEServers[0].URLs[0] != "turn:sfu.example.org:3478" {
		t.Fatal("explicit SFU ICE override ignored")
	}
	t.Setenv("MEDIA_ICE_SERVERS_JSON", `[]`)
	c, err = LoadMedia()
	if err != nil || len(c.ICE.ICEServers) != 0 {
		t.Fatal("explicit empty SFU override must disable fallback", err)
	}
	t.Setenv("MEDIA_ICE_SERVERS_JSON", `invalid`)
	if _, err = LoadMedia(); err == nil || !strings.Contains(err.Error(), "MEDIA_ICE_SERVERS_JSON") {
		t.Fatal("invalid explicit SFU override silently fell back")
	}
}
