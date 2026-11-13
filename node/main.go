package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/WittF/fnmdp/node/internal/assign"
	"github.com/WittF/fnmdp/node/internal/collector"
	"github.com/WittF/fnmdp/node/internal/config"
	"github.com/WittF/fnmdp/node/internal/persona"
	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/node/internal/scheduler"
	"github.com/WittF/fnmdp/node/internal/wsclient"
	"github.com/WittF/fnmdp/shared/getv2"
	"github.com/WittF/fnmdp/shared/httpx"
	"github.com/WittF/fnmdp/shared/protocol"
)

type stdoutSink struct{}

func (stdoutSink) Send(ctx context.Context, msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(b))
	return err
}

type minIntervalSource struct {
	runtime *config.Runtime
}

func (s minIntervalSource) MinInterval() time.Duration {
	return s.runtime.Snapshot().MinInterval
}

func main() {
	standalone := flag.Bool("standalone", false, "run without master websocket")
	projects := flag.String("projects", "", "comma-separated project ids for standalone mode")
	interval := flag.Duration("interval", 30*time.Second, "standalone polling interval")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	runtime := config.NewRuntime()
	runtime.SetHeartbeatInterval(cfg.HeartbeatInterval)
	mallRate := ratelimit.New(config.DefaultSoftBackoff, config.DefaultHardCooldown)
	getv2Rate := ratelimit.New(config.DefaultSoftBackoff, config.DefaultHardCooldown)
	store := assign.NewStore()
	nodePersona := persona.ForNode(cfg.NodeID)
	httpClient, err := httpx.NewClient(nodePersona)
	if err != nil {
		fmt.Fprintf(os.Stderr, "http client: %v\n", err)
		os.Exit(1)
	}
	bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, 8*time.Second)
	if err := httpx.BootstrapBuvid(bootstrapCtx, httpClient); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "buvid bootstrap skipped: %v\n", err)
	}
	cancelBootstrap()
	mall := getv2.NewMall(httpClient)
	getv2Client := getv2.New(httpClient)
	fetcher := collector.NewFallbackFetcher(mall, "mall", getv2Client, "getv2", mallRate, getv2Rate)
	fetcher.SetSecondaryMinInterval(cfg.GetV2FallbackMinInterval)

	var sink collector.Sink
	if *standalone {
		assignments, err := standaloneAssignments(*projects, *interval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "standalone config: %v\n", err)
			os.Exit(2)
		}
		store.Replace(assignments)
		sink = stdoutSink{}
		fmt.Fprintf(os.Stderr, "fnmdp node standalone: node_id=%s projects=%d interval=%s protocol=v%d\n",
			cfg.NodeID, len(assignments), interval.String(), protocol.Version)
	} else {
		ws := wsclient.New(cfg, store, runtime, mallRate)
		sink = ws
		go func() {
			if err := ws.Run(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "websocket stopped: %v\n", err)
				stop()
			}
		}()
		fmt.Fprintf(os.Stderr, "fnmdp node: node_id=%s master=%s protocol=v%d\n",
			cfg.NodeID, cfg.MasterURL, protocol.Version)
	}

	coll := collector.New(cfg.NodeID, fetcher, sink, mallRate, func() collector.ClientConfigSnapshot {
		snap := runtime.Snapshot()
		return collector.ClientConfigSnapshot{
			UserAgent:    snap.UserAgent,
			SoftBackoff:  snap.SoftBackoff,
			HardCooldown: snap.HardCooldown,
			Keepalive:    snap.Keepalive,
		}
	})
	sched := scheduler.New(store, minIntervalSource{runtime: runtime}, coll.Poll)
	if err := sched.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "scheduler stopped: %v\n", err)
		os.Exit(1)
	}
}

func standaloneAssignments(projects string, interval time.Duration) ([]protocol.Assignment, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("interval must be positive")
	}
	parts := strings.Split(projects, ",")
	projectIDs := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid project id %q", part)
		}
		projectIDs = append(projectIDs, id)
	}
	if len(projectIDs) == 0 {
		return nil, fmt.Errorf("--projects is required in standalone mode")
	}
	out := make([]protocol.Assignment, 0, len(projectIDs))
	phaseMod := len(projectIDs)
	for _, id := range projectIDs {
		out = append(out, protocol.Assignment{
			ProjectID:  id,
			Phase:      len(out),
			PhaseMod:   phaseMod,
			IntervalMs: int(interval / time.Millisecond),
		})
	}
	return out, nil
}
