package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/anycable/anycable-go/config"
	"github.com/anycable/anycable-go/version"
	"github.com/urfave/cli/v2"
)

// NewConfigFromCLI reads config from os.Args. It returns config, error (if any) and a bool value
// indicating that the usage message or version was shown, no further action required.
func NewConfigFromCLI(args []string) (*config.Config, error, bool) {
	c := config.NewConfig()

	var path, headers, cookieFilter string
	var helpOrVersionWereShown bool = true

	// Print raw version without prefix
	cli.VersionPrinter = func(cCtx *cli.Context) {
		_, _ = fmt.Fprintf(cCtx.App.Writer, "%v\n", cCtx.App.Version)
	}

	flags := []cli.Flag{}
	flags = append(flags, serverCLIFlags(&c, &path)...)
	flags = append(flags, sslCLIFlags(&c)...)
	flags = append(flags, broadcastCLIFlags(&c)...)
	flags = append(flags, redisCLIFlags(&c)...)
	flags = append(flags, httpBroadcastCLIFlags(&c)...)
	flags = append(flags, natsCLIFlags(&c)...)
	flags = append(flags, rpcCLIFlags(&c, &headers, &cookieFilter)...)
	flags = append(flags, disconnectorCLIFlags(&c)...)
	flags = append(flags, logCLIFlags(&c)...)
	flags = append(flags, metricsCLIFlags(&c)...)
	flags = append(flags, wsCLIFlags(&c)...)
	flags = append(flags, pingCLIFlags(&c)...)
	flags = append(flags, jwtCLIFlags(&c)...)
	flags = append(flags, signedStreamsCLIFlags(&c)...)

	app := &cli.App{
		Name:            "anycable-go",
		Version:         version.Version(),
		Usage:           "AnyCable-Go, The WebSocket server for https://anycable.io",
		HideHelpCommand: true,
		Flags:           flags,
		Action: func(nc *cli.Context) error {
			helpOrVersionWereShown = false
			return nil
		},
	}

	err := app.Run(args)
	if err != nil {
		return &config.Config{}, err, false
	}

	// helpOrVersionWereShown = false indicates that the default action has been run.
	// true means that help/version message was displayed.
	//
	// Unfortunately, cli module does not support another way of detecting if or which
	// command was run.
	if helpOrVersionWereShown {
		return &config.Config{}, nil, true
	}

	if path != "" {
		c.Path = strings.Split(path, " ")
	}

	c.Headers = strings.Split(strings.ToLower(headers), ",")

	if len(cookieFilter) > 0 {
		c.Cookies = strings.Split(cookieFilter, ",")
	}

	if c.Debug {
		c.LogLevel = "debug"
		c.LogFormat = "text"
	}

	if c.Metrics.Port == 0 {
		c.Metrics.Port = c.Port
	}

	if c.Metrics.LogInterval > 0 {
		fmt.Println(`DEPRECATION WARNING: metrics_log_interval option is deprecated
and will be deleted in the next major release of anycable-go.
Use metrics_rotate_interval instead.`)

		if c.Metrics.RotateInterval == 0 {
			c.Metrics.RotateInterval = c.Metrics.LogInterval
		}
	}

	return &c, nil, false
}

// Flags ordering issue: https://github.com/urfave/cli/pull/1430

const (
	serverCategoryDescription        = "ANYCABLE-GO SERVER:"
	sslCategoryDescription           = "SSL:"
	broadcastCategoryDescription     = "BROADCASTING:"
	redisCategoryDescription         = "REDIS:"
	httpBroadcastCategoryDescription = "HTTP BROADCAST:"
	natsCategoryDescription          = "NATS:"
	rpcCategoryDescription           = "RPC:"
	disconnectorCategoryDescription  = "DISCONNECTOR:"
	logCategoryDescription           = "LOG:"
	metricsCategoryDescription       = "METRICS:"
	wsCategoryDescription            = "WEBSOCKETS:"
	pingCategoryDescription          = "PING:"
	jwtCategoryDescription           = "JWT:"
	signedStreamsCategoryDescription = "SIGNED STREAMS:"

	envPrefix = "ANYCABLE_"
)

var (
	splitFlagName = regexp.MustCompile("[_-]")
)

// serverCLIFlags returns base server flags
func serverCLIFlags(c *config.Config, path *string) []cli.Flag {
	return withDefaults(serverCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "host",
			Value:       c.Host,
			Usage:       "Server host",
			Destination: &c.Host,
		},

		&cli.IntFlag{
			Name:        "port",
			Value:       c.Port,
			Usage:       "Server port",
			EnvVars:     []string{envPrefix + "PORT", "PORT"},
			Destination: &c.Port,
		},

		&cli.IntFlag{
			Name:        "max-conn",
			Usage:       "Limit simultaneous server connections (0 – without limit)",
			Destination: &c.MaxConn,
		},

		&cli.StringFlag{
			Name:        "path",
			Value:       strings.Join(c.Path, ","),
			Usage:       "WebSocket endpoint path (you can specify multiple paths using comma as separator)",
			Destination: path,
		},

		&cli.StringFlag{
			Name:        "health-path",
			Value:       c.HealthPath,
			Usage:       "HTTP health endpoint path",
			Destination: &c.HealthPath,
		},
	})
}

// sslCLIFlags returns SSL flags
func sslCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(sslCategoryDescription, []cli.Flag{
		&cli.PathFlag{
			Name:        "ssl_cert",
			Usage:       "SSL certificate path",
			Destination: &c.SSL.CertPath,
		},

		&cli.PathFlag{
			Name:        "ssl_key",
			Usage:       "SSL private key path",
			Destination: &c.SSL.KeyPath,
		},
	})
}

// broadcastCLIFlags returns broadcast_adapter flag
func broadcastCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(broadcastCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "broadcast_adapter",
			Usage:       "Broadcasting adapter to use (redis, http or nats)",
			Value:       c.BroadcastAdapter,
			Destination: &c.BroadcastAdapter,
		},

		&cli.IntFlag{
			Name:        "hub_gopool_size",
			Usage:       "The size of the goroutines pool to broadcast messages",
			Value:       c.App.HubGopoolSize,
			Destination: &c.App.HubGopoolSize,
		},
	})
}

func redisCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(redisCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "redis_url",
			Usage:       "Redis url",
			Value:       c.Redis.URL,
			EnvVars:     []string{envPrefix + "REDIS_URL", "REDIS_URL"},
			Destination: &c.Redis.URL,
		},

		&cli.StringFlag{
			Name:        "redis_channel",
			Usage:       "Redis channel for broadcasts",
			Value:       c.Redis.Channel,
			Destination: &c.Redis.Channel,
		},

		&cli.StringFlag{
			Name:        "redis_sentinels",
			Usage:       "Comma separated list of sentinel hosts, format: 'hostname:port,..'",
			Destination: &c.Redis.Sentinels,
		},

		&cli.IntFlag{
			Name:        "redis_sentinel_discovery_interval",
			Usage:       "Interval to rediscover sentinels in seconds",
			Value:       c.Redis.SentinelDiscoveryInterval,
			Destination: &c.Redis.SentinelDiscoveryInterval,
		},

		&cli.IntFlag{
			Name:        "redis_keepalive_interval",
			Usage:       "Interval to periodically ping Redis to make sure it's alive",
			Value:       c.Redis.KeepalivePingInterval,
			Destination: &c.Redis.KeepalivePingInterval,
		},

		&cli.BoolFlag{
			Name:        "redis_tls_verify",
			Usage:       "Verify Redis server TLS certificate (only if URL protocol is rediss://)",
			Value:       c.Redis.TLSVerify,
			Destination: &c.Redis.TLSVerify,
		},
	})
}

// httpBroadcastCLIFlags returns HTTP CLI flags
func httpBroadcastCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(httpBroadcastCategoryDescription, []cli.Flag{
		&cli.IntFlag{
			Name:        "http_broadcast_port",
			Usage:       "HTTP pub/sub server port",
			Value:       c.HTTPPubSub.Port,
			Destination: &c.HTTPPubSub.Port,
		},

		&cli.StringFlag{
			Name:        "http_broadcast_path",
			Usage:       "HTTP pub/sub endpoint path",
			Value:       c.HTTPPubSub.Path,
			Destination: &c.HTTPPubSub.Path,
		},

		&cli.StringFlag{
			Name:        "http_broadcast_secret",
			Usage:       "HTTP pub/sub authorization secret",
			Destination: &c.HTTPPubSub.Secret,
		},
	})
}

// natsCLIFlags returns NATS cli flags
func natsCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(natsCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "nats_servers",
			Usage:       "Comma separated list of NATS cluster servers",
			Value:       c.NATSPubSub.Servers,
			Destination: &c.NATSPubSub.Servers,
		},

		&cli.StringFlag{
			Name:        "nats_channel",
			Usage:       "NATS channel for broadcasts",
			Value:       c.NATSPubSub.Channel,
			Destination: &c.NATSPubSub.Channel,
		},

		&cli.BoolFlag{
			Name:        "nats_dont_randomize_servers",
			Usage:       "Pass this option to disable NATS servers randomization during (re-)connect",
			Destination: &c.NATSPubSub.DontRandomizeServers,
		}})

}

// rpcCLIFlags returns CLI flags for RPC
func rpcCLIFlags(c *config.Config, headers, cookieFilter *string) []cli.Flag {
	return withDefaults(rpcCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "rpc_host",
			Usage:       "RPC service address",
			Value:       c.RPC.Host,
			Destination: &c.RPC.Host,
		},

		&cli.IntFlag{
			Name:        "rpc_concurrency",
			Usage:       "Max number of concurrent RPC request; should be slightly less than the RPC server concurrency",
			Value:       c.RPC.Concurrency,
			Destination: &c.RPC.Concurrency,
		},

		&cli.BoolFlag{
			Name:        "rpc_enable_tls",
			Usage:       "Enable client-side TLS with the RPC server",
			Destination: &c.RPC.EnableTLS,
		},

		&cli.IntFlag{
			Name:        "rpc_max_call_recv_size",
			Usage:       "Override default MaxCallRecvMsgSize for RPC client (bytes)",
			Value:       c.RPC.MaxRecvSize,
			Destination: &c.RPC.MaxRecvSize,
		},

		&cli.IntFlag{
			Name:        "rpc_max_call_send_size",
			Usage:       "Override default MaxCallSendMsgSize for RPC client (bytes)",
			Value:       c.RPC.MaxSendSize,
			Destination: &c.RPC.MaxSendSize,
		},

		&cli.StringFlag{
			Name:        "headers",
			Usage:       "List of headers to proxy to RPC",
			Value:       strings.Join(c.Headers, ","),
			Destination: headers,
		},

		&cli.StringFlag{
			Name:        "proxy-cookies",
			Usage:       "Cookie keys to send to RPC, default is all",
			Destination: cookieFilter,
		},
	})
}

// rpcCLIFlags returns CLI flags for disconnect options
func disconnectorCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(disconnectorCategoryDescription, []cli.Flag{
		&cli.IntFlag{
			Name:        "disconnect_rate",
			Usage:       "Max number of Disconnect calls per second",
			Value:       c.DisconnectQueue.Rate,
			Destination: &c.DisconnectQueue.Rate,
		},

		&cli.IntFlag{
			Name:        "disconnect_timeout",
			Usage:       "Graceful shutdown timeouts (in seconds)",
			Value:       c.DisconnectQueue.ShutdownTimeout,
			Destination: &c.DisconnectQueue.ShutdownTimeout,
		},

		&cli.BoolFlag{
			Name:        "disable_disconnect",
			Usage:       "Disable calling Disconnect callback",
			Destination: &c.DisconnectorDisabled,
		},
	})
}

// rpcCLIFlags returns CLI flags for logging
func logCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(logCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "log_level",
			Usage:       "Set logging level (debug/info/warn/error/fatal)",
			Value:       c.LogLevel,
			Destination: &c.LogLevel,
		},

		&cli.StringFlag{
			Name:        "log_format",
			Usage:       "Set logging format (text/json)",
			Value:       c.LogFormat,
			Destination: &c.LogFormat,
		},

		&cli.BoolFlag{
			Name:        "debug",
			Usage:       "Enable debug mode (more verbose logging)",
			Destination: &c.Debug,
		},
	})
}

// metricsCLIFlags returns CLI flags for metrics
func metricsCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(metricsCategoryDescription, []cli.Flag{
		// Metrics
		&cli.BoolFlag{
			Name:        "metrics_log",
			Usage:       "Enable metrics logging (with info level)",
			Destination: &c.Metrics.Log,
		},

		&cli.IntFlag{
			Name:        "metrics_rotate_interval",
			Usage:       "Specify how often flush metrics to writers (logs, statsd) (in seconds)",
			Value:       c.Metrics.RotateInterval,
			Destination: &c.Metrics.RotateInterval,
		},

		&cli.IntFlag{
			Name:        "metrics_log_interval",
			Usage:       "DEPRECATED. Specify how often flush metrics logs (in seconds)",
			Value:       c.Metrics.LogInterval,
			Destination: &c.Metrics.LogInterval,
		},

		&cli.StringFlag{
			Name:        "metrics_log_formatter",
			Usage:       "Specify the path to custom Ruby formatter script (only supported on MacOS and Linux)",
			Destination: &c.Metrics.LogFormatter,
		},

		&cli.StringFlag{
			Name:        "metrics_http",
			Usage:       "Enable HTTP metrics endpoint at the specified path",
			Destination: &c.Metrics.HTTP,
		},

		&cli.StringFlag{
			Name:        "metrics_host",
			Usage:       "Server host for metrics endpoint",
			Destination: &c.Metrics.Host,
		},

		&cli.IntFlag{
			Name:        "metrics_port",
			Usage:       "Server port for metrics endpoint, the same as for main server by default",
			Destination: &c.Metrics.Port,
		},

		&cli.IntFlag{
			Name:        "stats_refresh_interval",
			Usage:       "How often to refresh the server stats (in seconds)",
			Value:       c.App.StatsRefreshInterval,
			Destination: &c.App.StatsRefreshInterval,
		},
	})
}

// wsCLIFlags returns CLI flags for WebSocket
func wsCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(wsCategoryDescription, []cli.Flag{
		&cli.IntFlag{
			Name:        "read_buffer_size",
			Usage:       "WebSocket connection read buffer size",
			Value:       c.WS.ReadBufferSize,
			Destination: &c.WS.ReadBufferSize,
		},

		&cli.IntFlag{
			Name:        "write_buffer_size",
			Usage:       "WebSocket connection write buffer size",
			Value:       c.WS.WriteBufferSize,
			Destination: &c.WS.WriteBufferSize,
		},

		&cli.Int64Flag{
			Name:        "max_message_size",
			Usage:       "Maximum size of a message in bytes",
			Value:       c.WS.MaxMessageSize,
			Destination: &c.WS.MaxMessageSize,
		},

		&cli.BoolFlag{
			Name:        "enable_ws_compression",
			Usage:       "Enable experimental WebSocket per message compression",
			Destination: &c.WS.EnableCompression,
		},

		&cli.StringFlag{
			Name:        "allowed_origins",
			Usage:       `Accept requests only from specified origins, e.g., "www.example.com,*example.io". No check is performed if empty`,
			Destination: &c.WS.AllowedOrigins,
		},
	})
}

// pingCLIFlags returns CLI flag for ping settings
func pingCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(pingCategoryDescription, []cli.Flag{
		&cli.IntFlag{
			Name:        "ping_interval",
			Usage:       "Action Cable ping interval (in seconds)",
			Value:       c.App.PingInterval,
			Destination: &c.App.PingInterval,
		},

		&cli.StringFlag{
			Name:        "ping_timestamp_precision",
			Usage:       "Precision for timestamps in ping messages (s, ms, ns)",
			Value:       c.App.PingTimestampPrecision,
			Destination: &c.App.PingTimestampPrecision,
		},
	})
}

// jwtCLIFlags returns CLI flags for JWT
func jwtCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(jwtCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "jwt_id_key",
			Usage:       "The encryption key used to verify JWT tokens",
			Destination: &c.JWT.Secret,
		},

		&cli.StringFlag{
			Name:        "jwt_id_param",
			Usage:       "The name of a query string param or an HTTP header carrying a token",
			Value:       c.JWT.Param,
			Destination: &c.JWT.Param,
		},

		&cli.BoolFlag{
			Name:        "jwt_id_enforce",
			Usage:       "Whether to enforce token presence for all connections",
			Destination: &c.JWT.Force,
		},
	})
}

// signedStreamsCLIFlags returns misc CLI flags
func signedStreamsCLIFlags(c *config.Config) []cli.Flag {
	return withDefaults(signedStreamsCategoryDescription, []cli.Flag{
		&cli.StringFlag{
			Name:        "turbo_rails_key",
			Usage:       "Enable Turbo Streams fastlane with the specified signing key",
			Destination: &c.Rails.TurboRailsKey,
		},

		&cli.StringFlag{
			Name:        "cable_ready_key",
			Usage:       "Enable CableReady fastlane with the specified signing key",
			Destination: &c.Rails.CableReadyKey,
		},
	})
}

// withDefaults sets category and env var name a flags passed as the arument
func withDefaults(category string, flags []cli.Flag) []cli.Flag {
	for _, f := range flags {
		switch v := f.(type) {
		case *cli.IntFlag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.Int64Flag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.Float64Flag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.DurationFlag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.BoolFlag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.StringFlag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.PathFlag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		case *cli.TimestampFlag:
			v.Category = category
			if len(v.EnvVars) == 0 {
				v.EnvVars = []string{nameToEnvVarName(v.Name)}
			}
		}
	}
	return flags
}

// nameToEnvVarName converts flag name to env variable
func nameToEnvVarName(name string) string {
	split := splitFlagName.Split(name, -1)
	set := []string{}

	for i := range split {
		set = append(set, strings.ToUpper(split[i]))
	}

	return envPrefix + strings.Join(set, "_")
}
