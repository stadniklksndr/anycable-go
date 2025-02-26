package config

import (
	"github.com/anycable/anycable-go/identity"
	"github.com/anycable/anycable-go/metrics"
	"github.com/anycable/anycable-go/node"
	"github.com/anycable/anycable-go/pubsub"
	"github.com/anycable/anycable-go/rails"
	"github.com/anycable/anycable-go/rpc"
	"github.com/anycable/anycable-go/server"
	"github.com/anycable/anycable-go/ws"
)

// Config contains main application configuration
type Config struct {
	App                  node.Config
	RPC                  rpc.Config
	Redis                pubsub.RedisConfig
	HTTPPubSub           pubsub.HTTPConfig
	NATSPubSub           pubsub.NATSConfig
	Host                 string
	Port                 int
	MaxConn              int
	BroadcastAdapter     string
	Path                 []string
	HealthPath           string
	Headers              []string
	Cookies              []string
	SSL                  server.SSLConfig
	WS                   ws.Config
	MaxMessageSize       int64
	DisconnectorDisabled bool
	DisconnectQueue      node.DisconnectQueueConfig
	LogLevel             string
	LogFormat            string
	Debug                bool
	Metrics              metrics.Config
	JWT                  identity.JWTConfig
	Rails                rails.Config
}

// NewConfig returns a new empty config
func NewConfig() Config {
	config := Config{
		Host:             "localhost",
		Port:             8080,
		Path:             []string{"/cable"},
		HealthPath:       "/health",
		BroadcastAdapter: "redis",
		Headers:          []string{"cookie"},
		LogLevel:         "info",
		LogFormat:        "text",
		App:              node.NewConfig(),
		SSL:              server.NewSSLConfig(),
		WS:               ws.NewConfig(),
		Metrics:          metrics.NewConfig(),
		RPC:              rpc.NewConfig(),
		Redis:            pubsub.NewRedisConfig(),
		HTTPPubSub:       pubsub.NewHTTPConfig(),
		NATSPubSub:       pubsub.NewNATSConfig(),
		DisconnectQueue:  node.NewDisconnectQueueConfig(),
		JWT:              identity.NewJWTConfig(""),
		Rails:            rails.NewConfig(),
	}

	return config
}
