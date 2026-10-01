package config

import (
	"testing"
	"time"
)

func TestRealtimeConfigValidation(t *testing.T) {
	c, err := LoadRealtime()
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RealtimeConfig){func(c *RealtimeConfig) { c.QueueSize = 0 }, func(c *RealtimeConfig) { c.TicketTTL = time.Minute + time.Second }, func(c *RealtimeConfig) { c.SessionTTL = time.Second }, func(c *RealtimeConfig) { c.MessageBytes = 0 }, func(c *RealtimeConfig) { c.AllowedOrigins = []string{"https://example.com/path"} }} {
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
