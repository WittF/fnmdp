package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/WittF/fnmdp/shared/protocol"
)

const (
	DefaultMasterURL                = "wss://master.example.com/ingest"
	DefaultRegion                   = "unknown"
	DefaultMinInterval              = 1100 * time.Millisecond
	DefaultSoftBackoff              = time.Second
	DefaultHardCooldown             = 10 * time.Minute
	DefaultHeartbeatInterval        = 12 * time.Second
	DefaultGetV2FallbackMinInterval = 15 * time.Second
	// DefaultKeepalive forces a floor poll_result at least this often even when the
	// SKU state is unchanged, so per-project liveness keeps advancing and the
	// frontend timeline stays continuous. Master overrides via Config.KeepaliveMs.
	DefaultKeepalive = 90 * time.Second
)

type Config struct {
	MasterURL                string
	NodeID                   string
	NodeName                 string
	Region                   string
	NodeToken                string
	HeartbeatInterval        time.Duration
	GetV2FallbackMinInterval time.Duration
}

func Load() Config {
	hostname, _ := os.Hostname()
	nodeID := getenv("NODE_ID", hostname)
	if nodeID == "" {
		nodeID = "ephemeral-" + randomHex(8)
	}
	nodeName := getenv("NODE_NAME", hostname)
	if nodeName == "" {
		nodeName = nodeID
	}
	return Config{
		MasterURL:                getenv("MASTER_URL", DefaultMasterURL),
		NodeID:                   nodeID,
		NodeName:                 nodeName,
		Region:                   getenv("REGION", DefaultRegion),
		NodeToken:                os.Getenv("NODE_TOKEN"),
		HeartbeatInterval:        envDuration("HEARTBEAT_INTERVAL", DefaultHeartbeatInterval),
		GetV2FallbackMinInterval: envDuration("GETV2_FALLBACK_MIN_INTERVAL_MS", DefaultGetV2FallbackMinInterval),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
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

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

type Runtime struct {
	mu             sync.RWMutex
	minInterval    time.Duration
	softBackoff    time.Duration
	hardCooldown   time.Duration
	userAgent      string
	heartbeatEvery time.Duration
	keepalive      time.Duration
}

func NewRuntime() *Runtime {
	return &Runtime{
		minInterval:    DefaultMinInterval,
		softBackoff:    DefaultSoftBackoff,
		hardCooldown:   DefaultHardCooldown,
		heartbeatEvery: DefaultHeartbeatInterval,
		keepalive:      DefaultKeepalive,
	}
}

func (r *Runtime) ApplyConfig(msg protocol.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg.MinIntervalMs > 0 {
		r.minInterval = time.Duration(msg.MinIntervalMs) * time.Millisecond
	}
	if msg.SoftBackoffMs > 0 {
		r.softBackoff = time.Duration(msg.SoftBackoffMs) * time.Millisecond
	}
	if msg.HardCooldownMs > 0 {
		r.hardCooldown = time.Duration(msg.HardCooldownMs) * time.Millisecond
	}
	if msg.UserAgent != "" {
		r.userAgent = msg.UserAgent
	}
	if msg.KeepaliveMs > 0 {
		r.keepalive = time.Duration(msg.KeepaliveMs) * time.Millisecond
	}
}

func (r *Runtime) SetHeartbeatInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	r.mu.Lock()
	r.heartbeatEvery = d
	r.mu.Unlock()
}

func (r *Runtime) Snapshot() RuntimeSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RuntimeSnapshot{
		MinInterval:    r.minInterval,
		SoftBackoff:    r.softBackoff,
		HardCooldown:   r.hardCooldown,
		UserAgent:      r.userAgent,
		HeartbeatEvery: r.heartbeatEvery,
		Keepalive:      r.keepalive,
	}
}

type RuntimeSnapshot struct {
	MinInterval    time.Duration
	SoftBackoff    time.Duration
	HardCooldown   time.Duration
	UserAgent      string
	HeartbeatEvery time.Duration
	Keepalive      time.Duration
}
