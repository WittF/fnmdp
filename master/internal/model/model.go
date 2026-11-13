package model

import "time"

type Project struct {
	ID               int64     `json:"project_id"`
	Name             string    `json:"name"`
	Flag             int       `json:"flag"`
	FlagText         string    `json:"flag_text"`
	Hot              bool      `json:"hot"`
	WishCount        int64     `json:"wish_count"`
	PriceLow         int       `json:"price_low"`
	PriceHigh        int       `json:"price_high"`
	SalableLow       int       `json:"salable_low"`
	SalableHigh      int       `json:"salable_high"`
	Status           int       `json:"status"`
	IsSale           int       `json:"is_sale"`
	SaleStart        int64     `json:"sale_start"`
	SaleEnd          int64     `json:"sale_end"`
	CountDown        int64     `json:"count_down"`
	ServerTs         int64     `json:"server_ts"`
	LastEventTs      int64     `json:"last_event_ts"`
	LastPolledTs     int64     `json:"last_poll_ts"`
	Priority         int       `json:"priority"`
	TargetIntervalMs int       `json:"target_interval_ms"`
	ReplicaFactor    int       `json:"replica_factor"`
	Enabled          bool      `json:"enabled"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Screen struct {
	ProjectID   int64     `json:"project_id"`
	ScreenID    int64     `json:"screen_id"`
	Name        string    `json:"name"`
	StartTime   int64     `json:"start_time"`
	Flag        int       `json:"flag"`
	FlagText    string    `json:"flag_text"`
	Clickable   bool      `json:"clickable"`
	SaleStart   int64     `json:"sale_start"`
	SaleEnd     int64     `json:"sale_end"`
	ServerTs    int64     `json:"server_ts"`
	LastEventTs int64     `json:"last_event_ts"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SKU struct {
	ProjectID   int64     `json:"project_id"`
	ScreenID    int64     `json:"screen_id"`
	SkuID       int64     `json:"sku_id"`
	Desc        string    `json:"desc"`
	Flag        int       `json:"flag"`
	FlagText    string    `json:"flag_text"`
	Clickable   bool      `json:"clickable"`
	PriceFen    int       `json:"price_fen"`
	Num         int       `json:"num"`
	LimitNum    int       `json:"limit_num"`
	ServerTs    int64     `json:"server_ts"`
	LastEventTs int64     `json:"last_event_ts"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Event struct {
	ID           int64          `json:"id,omitempty"`
	ProjectID    int64          `json:"project_id"`
	ScreenID     int64          `json:"screen_id"`
	SkuID        int64          `json:"sku_id"`
	EventType    string         `json:"event_type"`
	Severity     string         `json:"severity"`
	OldFlag      int            `json:"old_flag"`
	NewFlag      int            `json:"new_flag"`
	OldClickable bool           `json:"old_clickable"`
	NewClickable bool           `json:"new_clickable"`
	OldPriceFen  int            `json:"old_price_fen"`
	PriceFen     int            `json:"price_fen"`
	OldNum       int            `json:"old_num"`
	NewNum       int            `json:"new_num"`
	FlagText     string         `json:"flag_text"`
	ServerTs     int64          `json:"server_ts"`
	NodeID       string         `json:"node_id"`
	Details      map[string]any `json:"details,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type PollCoverage struct {
	ProjectID int64 `json:"project_id"`
	StartTs   int64 `json:"start_ts"`
	EndTs     int64 `json:"end_ts"`
}

type TrajectoryState struct {
	Flag      int    `json:"flag"`
	Clickable bool   `json:"clickable"`
	FlagText  string `json:"flag_text"`
}

type TrajectoryPoint struct {
	T         int64             `json:"t"` // bucket start, epoch milliseconds
	Flag      int               `json:"flag"`
	Clickable bool              `json:"clickable"`
	FlagText  string            `json:"flag_text"`
	Observed  bool              `json:"observed"`
	Samples   []TrajectoryState `json:"samples,omitempty"`
}

type TrajectorySeries struct {
	ProjectID int64             `json:"project_id"`
	ScreenID  int64             `json:"screen_id"`
	SkuID     int64             `json:"sku_id"`
	Points    []TrajectoryPoint `json:"points"`
}

type Node struct {
	NodeID       string    `json:"node_id"`
	NodeName     string    `json:"node_name"`
	Region       string    `json:"region"`
	Online       bool      `json:"online"`
	InFlight     int       `json:"in_flight"`
	SoftLimited  int       `json:"soft_limited"`
	HardBlocked  int       `json:"hard_blocked"`
	LastSeen     int64     `json:"last_seen"`
	SkewMs       int64     `json:"skew_ms"`
	BlockedUntil time.Time `json:"blocked_until,omitempty"`
	BudgetRPS    float64   `json:"budget_rps,omitempty"`
	Assignments  []int64   `json:"assignments"`
}

type Snapshot struct {
	Projects []Project `json:"projects"`
	Screens  []Screen  `json:"screens"`
	SKUs     []SKU     `json:"skus"`
	Nodes    []Node    `json:"nodes"`
}
