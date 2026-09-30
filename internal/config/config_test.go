package config

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQURL(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want amqp.URI
	}{
		{
			name: "local broker defaults",
			want: amqp.URI{Scheme: "amqp", Host: "rabbitmq", Port: 5672, Username: "go_recorder", Password: "go_recorder_pass", Vhost: "/"},
		},
		{
			name: "host process and custom vhost",
			env: map[string]string{
				"RABBIT_MQ_HOST": "127.0.0.1", "RABBIT_MQ_PORT": "5673",
				"RABBIT_MQ_USER": "recorder", "RABBIT_MQ_PASSWORD": "secret", "RABBIT_MQ_VHOST": "recordings",
			},
			want: amqp.URI{Scheme: "amqp", Host: "127.0.0.1", Port: 5673, Username: "recorder", Password: "secret", Vhost: "recordings"},
		},
		{
			name: "credentials and vhost retain special characters",
			env: map[string]string{
				"RABBIT_MQ_USER": "record user+@", "RABBIT_MQ_PASSWORD": "pass word+/@:#%",
				"RABBIT_MQ_VHOST": "/recordings/team one",
			},
			want: amqp.URI{Scheme: "amqp", Host: "rabbitmq", Port: 5672, Username: "record user+@", Password: "pass word+/@:#%", Vhost: "/recordings/team one"},
		},
		{
			name: "IPv6 host",
			env:  map[string]string{"RABBIT_MQ_HOST": "::1"},
			want: amqp.URI{Scheme: "amqp", Host: "::1", Port: 5672, Username: "go_recorder", Password: "go_recorder_pass", Vhost: "/"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"RABBIT_MQ_DSN", "RABBIT_MQ_HOST", "RABBIT_MQ_PORT", "RABBIT_MQ_USER", "RABBIT_MQ_PASSWORD", "RABBIT_MQ_VHOST"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			got, err := amqp.ParseURI(rabbitMQURL())
			if err != nil {
				t.Fatalf("parse RabbitMQ URL: %v", err)
			}
			if got.Scheme != tt.want.Scheme || got.Host != tt.want.Host || got.Port != tt.want.Port ||
				got.Username != tt.want.Username || got.Password != tt.want.Password || got.Vhost != tt.want.Vhost {
				t.Fatalf("parsed URI = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRabbitMQURLExplicitDSN(t *testing.T) {
	const dsn = "amqps://custom:secret@broker.example:5671/recordings"
	t.Setenv("RABBIT_MQ_DSN", dsn)
	t.Setenv("RABBIT_MQ_HOST", "rabbitmq")
	if got := rabbitMQURL(); got != dsn {
		t.Fatalf("RabbitMQ URL = %q, want explicit DSN", got)
	}
}
