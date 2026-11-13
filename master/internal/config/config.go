package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultListenAddr       = ":8080"
	DefaultIntervalMs       = 30000
	DefaultHotIntervalMs    = 10000
	DefaultReplicaFactor    = 1
	DefaultHotMaxReplica    = 0
	DefaultNodeBudgetRPS    = 0.9
	DefaultMinIntervalMs    = 1100
	DefaultSoftBackoffMs    = 1000
	DefaultHardCooldownMs   = 600000
	DefaultKeepaliveMs      = 90000
	DefaultHeartbeatTTL     = 36 * time.Second
	DefaultPGMaxConns       = 4
	DefaultPGOpTimeout      = 5 * time.Second
	DefaultHistoryRetention = 14 * 24 * time.Hour
	DefaultCoverageGap      = 5 * time.Minute
)

type Config struct {
	ListenAddr string
	NodeToken  string
	AdminToken string

	InitialProjects []int64

	DefaultIntervalMs    int
	DefaultHotIntervalMs int
	DefaultReplicaFactor int
	HotMaxReplica        int
	NodeBudgetRPS        float64
	MinIntervalMs        int
	SoftBackoffMs        int
	HardCooldownMs       int
	KeepaliveMs          int
	UserAgent            string
	HeartbeatTimeout     time.Duration
	HistoryRetention     time.Duration
	CoverageGap          time.Duration

	PGHost      string
	PGPort      int
	PGUser      string
	PGPassword  string
	PGDatabase  string
	PGSSLMode   string
	PGMaxConns  int
	PGOpTimeout time.Duration

	ValkeyAddr     string
	ValkeyPassword string
	ValkeyDB       int
	ValkeyTLS      bool
}

func Load() Config {
	return Config{
		ListenAddr: getenv("LISTEN_ADDR", DefaultListenAddr),
		NodeToken:  os.Getenv("NODE_TOKEN"),
		AdminToken: os.Getenv("ADMIN_TOKEN"),

		InitialProjects: parseIDs(os.Getenv("INITIAL_PROJECTS")),

		DefaultIntervalMs:    envInt("DEFAULT_INTERVAL_MS", DefaultIntervalMs),
		DefaultHotIntervalMs: envInt("HOT_INTERVAL_MS", DefaultHotIntervalMs),
		DefaultReplicaFactor: envInt("DEFAULT_REPLICA_FACTOR", DefaultReplicaFactor),
		HotMaxReplica:        envInt("HOT_MAX_REPLICA", DefaultHotMaxReplica),
		NodeBudgetRPS:        envFloat("NODE_BUDGET_RPS", DefaultNodeBudgetRPS),
		MinIntervalMs:        envInt("MIN_INTERVAL_MS", DefaultMinIntervalMs),
		SoftBackoffMs:        envInt("SOFT_BACKOFF_MS", DefaultSoftBackoffMs),
		HardCooldownMs:       envInt("HARD_COOLDOWN_MS", DefaultHardCooldownMs),
		KeepaliveMs:          envInt("KEEPALIVE_MS", DefaultKeepaliveMs),
		UserAgent:            os.Getenv("USER_AGENT"),
		HeartbeatTimeout:     envDuration("HEARTBEAT_TIMEOUT", DefaultHeartbeatTTL),
		HistoryRetention:     envDuration("HISTORY_RETENTION", DefaultHistoryRetention),
		CoverageGap:          envDuration("COVERAGE_GAP", DefaultCoverageGap),

		PGHost:      os.Getenv("PGHOST"),
		PGPort:      envInt("PGPORT", 5432),
		PGUser:      os.Getenv("PGUSER"),
		PGPassword:  os.Getenv("PGPASSWORD"),
		PGDatabase:  getenv("PGDATABASE", "postgres"),
		PGSSLMode:   getenv("PGSSLMODE", "require"),
		PGMaxConns:  envInt("PGMAXCONNS", DefaultPGMaxConns),
		PGOpTimeout: envDuration("PGOPTIMEOUT", DefaultPGOpTimeout),

		ValkeyAddr:     os.Getenv("VALKEY_ADDR"),
		ValkeyPassword: os.Getenv("VALKEY_PASSWORD"),
		ValkeyDB:       envInt("VALKEY_DB", 0),
		ValkeyTLS:      envBool("VALKEY_TLS", false),
	}
}

func (c Config) PGEnabled() bool {
	return c.PGHost != "" && c.PGUser != "" && c.PGPassword != ""
}

func (c Config) ValkeyEnabled() bool {
	return c.ValkeyAddr != ""
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(v, "d")))
		if err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	d, err := time.ParseDuration(v)
	if err == nil {
		return d
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func parseIDs(v string) []int64 {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}
