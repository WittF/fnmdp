package valkey

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
)

type Window struct {
	client *redis.Client
}

type Config struct {
	Addr     string
	Password string
	DB       int
	TLS      bool
}

func New(ctx context.Context, cfg Config) (*Window, error) {
	options := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Window{client: client}, nil
}

func (w *Window) SeenOrMark(ctx context.Context, key string, ttl time.Duration) bool {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	ok, err := w.client.SetNX(ctx, key, "1", ttl).Result()
	return err != nil || !ok
}

func (w *Window) Close() error {
	return w.client.Close()
}
