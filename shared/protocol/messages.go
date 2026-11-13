package protocol

// Severity ranks an event for the frontend. Mirrors schema.json#severities.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityNotable  Severity = "notable"
	SeverityInfo     Severity = "info"
	SeverityOps      Severity = "ops"
)

// RateKind classifies a rate-control hit. Mirrors schema.json#rate_kinds.
type RateKind string

const (
	RateSoftLimited RateKind = "soft_limited" // HTTP 429
	RateHardBlocked RateKind = "hard_blocked" // HTTP 412
)

// EventType is an open enum; detection lives in the master state machine. Seed list mirrors
// schema.json#event_types.
type EventType string

const (
	// SKU level.
	EventSkuRestock         EventType = "sku_restock"
	EventSkuOnsale          EventType = "sku_onsale"
	EventSkuAdded           EventType = "sku_added"
	EventSkuSoldout         EventType = "sku_soldout"
	EventSkuPriceChange     EventType = "sku_price_change"
	EventSkuThresholdChange EventType = "sku_threshold_change"
	EventSkuLowStock        EventType = "sku_low_stock"
	EventSkuRemoved         EventType = "sku_removed"
	// Screen level.
	EventScreenOnsale  EventType = "screen_onsale"
	EventScreenSoldout EventType = "screen_soldout"
	EventScreenAdded   EventType = "screen_added"
	EventScreenRemoved EventType = "screen_removed"
	EventDaySoldout    EventType = "day_soldout"
	// Project level.
	EventProjectHotOn           EventType = "project_hot_on"
	EventProjectHotOff          EventType = "project_hot_off"
	EventProjectOnsale          EventType = "project_onsale"
	EventProjectPresale         EventType = "project_presale"
	EventProjectSoldout         EventType = "project_soldout"
	EventProjectWindowOpen      EventType = "project_window_open"
	EventProjectPriceBandChange EventType = "project_price_band_change"
	EventProjectStatusChange    EventType = "project_status_change"
	EventWishMilestone          EventType = "wish_milestone"
	// Ops.
	EventRisk412        EventType = "risk_412"
	EventRisk429        EventType = "risk_429"
	EventNodeOnline     EventType = "node_online"
	EventNodeOffline    EventType = "node_offline"
	EventMonitorAdded   EventType = "monitor_added"
	EventMonitorRemoved EventType = "monitor_removed"
)

// Message type discriminators.
const (
	TypeRegister    = "register"
	TypeHeartbeat   = "heartbeat"
	TypePollResult  = "poll_result"
	TypeRateEvent   = "rate_event"
	TypeAssign      = "assign"
	TypeAssignDelta = "assign_delta"
	TypeConfig      = "config"
	TypeSnapshot    = "snapshot"
	TypeStateUpdate = "state_update"
	TypeEvent       = "event"
	TypeNodeStatus  = "node_status"
)

// Fingerprint is the change-significant SKU state carried on state updates and
// events (mirror of state.StateFingerprint, kept here so wire types don't import
// the dedup package).
type Fingerprint struct {
	Flag      int  `json:"flag"`
	Clickable bool `json:"clickable"`
	PriceFen  int  `json:"price_fen"`
	// Num is the available count = min(real stock, per-order limit). 0 = sold out
	// (or not yet on sale; tell apart by Flag). Below limit_num it is the exact
	// remaining count — the highest-value early sold-out / restock signal (spec §5.1).
	Num int `json:"num"`
}

// Assignment is one project polling lease from master to node.
type Assignment struct {
	ProjectID  int64 `json:"project_id"`
	Phase      int   `json:"phase"`
	PhaseMod   int   `json:"phase_mod"`
	IntervalMs int   `json:"interval_ms"`
}

// SkuDiff is one changed SKU inside a poll_result.
type SkuDiff struct {
	ScreenID int64  `json:"screen_id"`
	SkuID    int64  `json:"sku_id"`
	Desc     string `json:"desc"`
	FlagText string `json:"flag_text"`
	// LimitNum is the per-order purchase cap (static_limit.num); near-constant. The
	// frontend derives 充足 (num>=limit) vs 告急 (0<num<limit) from num vs limit_num.
	LimitNum int          `json:"limit_num"`
	Old      *Fingerprint `json:"old"`
	New      *Fingerprint `json:"new"`
}

// ProjectMeta is the full current project-level metadata attached to every
// poll_result that L1 decides is worth sending.
type ProjectMeta struct {
	Name        string `json:"name"`
	Flag        int    `json:"flag"`
	FlagText    string `json:"flag_text"`
	Hot         bool   `json:"hot"`
	WishCount   int64  `json:"wish_count"`
	PriceLow    int    `json:"price_low"`
	PriceHigh   int    `json:"price_high"`
	SalableLow  int    `json:"salable_low"`
	SalableHigh int    `json:"salable_high"`
	Status      int    `json:"status"`
	IsSale      int    `json:"is_sale"`
	SaleStart   int64  `json:"sale_start"`
	SaleEnd     int64  `json:"sale_end"`
	CountDown   int64  `json:"count_down"`
	ServerTs    int64  `json:"server_ts"`
}

// ScreenDiff is the current state of a changed screen. Removed screens carry
// only their identity plus Removed=true.
type ScreenDiff struct {
	ScreenID  int64  `json:"screen_id"`
	Name      string `json:"name"`
	StartTime int64  `json:"start_time"`
	Flag      int    `json:"flag"`
	FlagText  string `json:"flag_text"`
	Clickable bool   `json:"clickable"`
	SaleStart int64  `json:"sale_start"`
	SaleEnd   int64  `json:"sale_end"`
	Removed   bool   `json:"removed"`
}

// ── node → master ──────────────────────────────────────────────────────────

// Register announces a node and presents the shared NODE_TOKEN.
type Register struct {
	Type         string   `json:"type"`
	Version      int      `json:"version"`
	NodeID       string   `json:"node_id"`
	NodeName     string   `json:"node_name"`
	Region       string   `json:"region"`
	Capabilities []string `json:"capabilities"`
}

// Heartbeat reports liveness and rate-control counters.
type Heartbeat struct {
	Type        string `json:"type"`
	Version     int    `json:"version"`
	NodeID      string `json:"node_id"`
	Ts          int64  `json:"ts"`
	InFlight    int    `json:"in_flight"`
	SoftLimited int    `json:"soft_limited"`
	HardBlocked int    `json:"hard_blocked"`
}

// PollResult carries the normalized diff of one getV2 poll.
type PollResult struct {
	Type      string       `json:"type"`
	Version   int          `json:"version"`
	NodeID    string       `json:"node_id"`
	ProjectID int64        `json:"project_id"`
	Source    string       `json:"source,omitempty"`
	ServerTs  int64        `json:"server_ts"`
	FetchedAt int64        `json:"fetched_at"`
	Diff      []SkuDiff    `json:"diff"`
	Project   *ProjectMeta `json:"project"`
	Screens   []ScreenDiff `json:"screens"`
}

// RateEvent reports a 429/412 hit, with the Gaia x-sec-request-id when present.
type RateEvent struct {
	Type         string   `json:"type"`
	Version      int      `json:"version"`
	NodeID       string   `json:"node_id"`
	ProjectID    int64    `json:"project_id"`
	StatusCode   int      `json:"status_code"`
	Kind         RateKind `json:"kind"`
	SecRequestID string   `json:"sec_request_id"`
	Ts           int64    `json:"ts"`
}

// ── master → node ──────────────────────────────────────────────────────────

// Assign tells a node to poll a project at a phase offset + interval.
type Assign struct {
	Type        string       `json:"type"`
	Version     int          `json:"version"`
	NodeID      string       `json:"node_id"`
	Assignments []Assignment `json:"assignments"`
}

// AssignDelta is an incremental assignment change.
type AssignDelta struct {
	Type    string       `json:"type"`
	Version int          `json:"version"`
	NodeID  string       `json:"node_id"`
	Added   []Assignment `json:"added"`
	Removed []int64      `json:"removed"`
}

// Config pushes global rate discipline to a node.
type Config struct {
	Type           string `json:"type"`
	Version        int    `json:"version"`
	MinIntervalMs  int    `json:"min_interval_ms"`
	SoftBackoffMs  int    `json:"soft_backoff_ms"`
	HardCooldownMs int    `json:"hard_cooldown_ms"`
	UserAgent      string `json:"user_agent"`
	// KeepaliveMs forces a floor poll_result at least this often even when the SKU
	// state is unchanged, so the master can advance per-project liveness and the
	// frontend timeline stays continuous. 0 = use the node default.
	KeepaliveMs int `json:"keepalive_ms"`
}

// ── master → frontend ──────────────────────────────────────────────────────

// Snapshot is the full current state sent on connect.
type Snapshot struct {
	Type     string `json:"type"`
	Version  int    `json:"version"`
	ServerTs int64  `json:"server_ts"`
	Projects []any  `json:"projects"`
	Nodes    []any  `json:"nodes"`
}

// StateUpdate is one SKU's current authoritative state.
type StateUpdate struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ProjectID int64  `json:"project_id"`
	ScreenID  int64  `json:"screen_id"`
	SkuID     int64  `json:"sku_id"`
	Flag      int    `json:"flag"`
	FlagText  string `json:"flag_text"`
	Clickable bool   `json:"clickable"`
	PriceFen  int    `json:"price_fen"`
	Num       int    `json:"num"`
	LimitNum  int    `json:"limit_num"`
	ServerTs  int64  `json:"server_ts"`
}

// Event is a classified real-time event with old/new fingerprints.
type Event struct {
	Type      string         `json:"type"`
	Version   int            `json:"version"`
	EventType EventType      `json:"event_type"`
	Severity  Severity       `json:"severity"`
	ProjectID int64          `json:"project_id"`
	ScreenID  int64          `json:"screen_id"`
	SkuID     int64          `json:"sku_id"`
	Old       *Fingerprint   `json:"old"`
	New       *Fingerprint   `json:"new"`
	PriceFen  int            `json:"price_fen"`
	FlagText  string         `json:"flag_text"`
	NodeID    string         `json:"node_id"`
	Details   map[string]any `json:"details,omitempty"`
	ServerTs  int64          `json:"server_ts"`
}

// NodeStatus is a node's online/health status for the frontend.
type NodeStatus struct {
	Type        string `json:"type"`
	Version     int    `json:"version"`
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	Region      string `json:"region"`
	Online      bool   `json:"online"`
	InFlight    int    `json:"in_flight"`
	SoftLimited int    `json:"soft_limited"`
	HardBlocked int    `json:"hard_blocked"`
	LastSeen    int64  `json:"last_seen"`
	// SkewMs is the master-observed node clock skew (master_recv_ms - node_ts_ms)
	// from the last heartbeat. Diagnostic only; liveness uses the master clock.
	SkewMs int64 `json:"skew_ms"`
}
