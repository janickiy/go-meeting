package config

import (
	"strings"
	"testing"
	"time"
)

func mediaTestEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "WEBRTC_ICE_SERVERS_JSON", "MEDIA_ICE_SERVERS_JSON", "MEDIA_UDP_MIN_PORT", "MEDIA_UDP_MAX_PORT", "MEDIA_UDP_PORT", "MEDIA_NAT_IPS", "MEDIA_WORKER_INTERNAL_URL", "MEDIA_WORKER_ID", "MEDIA_MAX_PEERS", "MEDIA_MAX_PUBLISHED_TRACKS", "MEDIA_TICKET_TTL", "MEDIA_HEARTBEAT_INTERVAL", "MEDIA_WORKER_TTL", "MEDIA_OWNERSHIP_TTL"} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "local")
	t.Setenv("JWT_SECRET", strings.Repeat("test-key-", 8))
}

func TestMediaConfigDefaultsAndLocalKeySeparation(t *testing.T) {
	mediaTestEnvironment(t)
	c, err := LoadMedia()
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxPeers != 10 || c.MaxPublishedTracks != 2 || c.VideoMaxWidth != 1280 || c.VideoMaxHeight != 720 || c.VideoMaxFPS != 30 || c.UDPPort == 50000 {
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
		{"ticket ttl", func(c *MediaConfig) { c.TicketTTL = time.Minute + time.Second }},
		{"heartbeat lease", func(c *MediaConfig) { c.OwnershipTTL = c.HeartbeatInterval }},
		{"worker lease", func(c *MediaConfig) { c.WorkerTTL = c.HeartbeatInterval }},
		{"room flooding", func(c *MediaConfig) { c.MaxPeers = 100 }},
		{"track flooding", func(c *MediaConfig) { c.MaxPublishedTracks = 100 }},
		{"unsupported audio slots", func(c *MediaConfig) { c.MaxAudioTracks = 2 }},
		{"unsupported video slots", func(c *MediaConfig) { c.MaxVideoTracks = 2 }},
		{"oversized video", func(c *MediaConfig) { c.VideoMaxWidth = 1920 }},
		{"endpoint credentials", func(c *MediaConfig) { c.WorkerInternalURL = "http://user:secret@worker:8091" }},
		{"endpoint query", func(c *MediaConfig) { c.WorkerInternalURL = "http://worker:8091/?secret=x" }},
		{"partial UDP range", func(c *MediaConfig) { c.UDPPort = 0; c.UDPMinPort = 51000 }},
		{"mux and range", func(c *MediaConfig) { c.UDPMinPort = 51000; c.UDPMaxPort = 51020 }},
		{"invalid public address", func(c *MediaConfig) { c.NATIPs = []string{"example.org"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.change(&c)
			if c.Validate() == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
}

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
