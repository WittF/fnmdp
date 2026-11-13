package config

import (
	"testing"
	"time"
)

func TestLoadPGMaxConns(t *testing.T) {
	t.Setenv("PGMAXCONNS", "3")

	cfg := Load()
	if cfg.PGMaxConns != 3 {
		t.Fatalf("PGMaxConns = %d, want 3", cfg.PGMaxConns)
	}
}

func TestLoadPGMaxConnsDefault(t *testing.T) {
	t.Setenv("PGMAXCONNS", "")

	cfg := Load()
	if cfg.PGMaxConns != DefaultPGMaxConns {
		t.Fatalf("PGMaxConns = %d, want %d", cfg.PGMaxConns, DefaultPGMaxConns)
	}
}

func TestLoadRetentionAndCoverageGap(t *testing.T) {
	t.Setenv("HISTORY_RETENTION", "2d")
	t.Setenv("COVERAGE_GAP", "2m")

	cfg := Load()
	if cfg.HistoryRetention != 48*time.Hour {
		t.Fatalf("HistoryRetention = %s, want 2d", cfg.HistoryRetention)
	}
	if cfg.CoverageGap != 2*time.Minute {
		t.Fatalf("CoverageGap = %s, want 2m", cfg.CoverageGap)
	}
}
