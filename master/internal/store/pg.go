package store

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type PGConfig struct {
	Host      string
	Port      int
	User      string
	Password  string
	Database  string
	SSLMode   string
	MaxConns  int
	OpTimeout time.Duration
}

type PG struct {
	pool      *pgxpool.Pool
	opTimeout time.Duration
}

const (
	defaultPGConnectTimeout = 5 * time.Second
	defaultPGOpTimeout      = 5 * time.Second
	defaultPGMaxLifetime    = 30 * time.Minute
	defaultPGMaxIdleTime    = 5 * time.Minute
	defaultPGHealthPeriod   = time.Minute
)

func NewPG(ctx context.Context, cfg PGConfig) (*PG, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		quoteKV(cfg.Host), cfg.Port, quoteKV(cfg.User), quoteKV(cfg.Password),
		quoteKV(cfg.Database), quoteKV(cfg.SSLMode))
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if cfg.MaxConns > 0 {
		pc.MaxConns = int32(cfg.MaxConns)
		pc.MinConns = 0
	}
	pc.ConnConfig.ConnectTimeout = defaultPGConnectTimeout
	pc.MaxConnLifetime = defaultPGMaxLifetime
	pc.MaxConnIdleTime = defaultPGMaxIdleTime
	pc.HealthCheckPeriod = defaultPGHealthPeriod
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	opTimeout := cfg.OpTimeout
	if opTimeout <= 0 {
		opTimeout = defaultPGOpTimeout
	}
	pg := &PG{pool: pool, opTimeout: opTimeout}
	if err := pg.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pg, nil
}

func (p *PG) EnsureProject(ctx context.Context, id int64) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
		INSERT INTO projects (bili_project_id, enabled, updated_at)
		VALUES ($1, true, now())
		ON CONFLICT (bili_project_id) DO NOTHING`, id)
	return err
}

func (p *PG) UpsertProject(ctx context.Context, m model.Project) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
		INSERT INTO projects (
			bili_project_id,name,flag,flag_text,hot,wish_count,price_low,price_high,
			salable_low,salable_high,status,is_sale,sale_start,sale_end,count_down,
			server_ts,priority,target_interval_ms,replica_factor,enabled,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,now())
		ON CONFLICT (bili_project_id) DO UPDATE SET
			name=EXCLUDED.name, flag=EXCLUDED.flag, flag_text=EXCLUDED.flag_text,
			hot=EXCLUDED.hot, wish_count=EXCLUDED.wish_count, price_low=EXCLUDED.price_low,
			price_high=EXCLUDED.price_high, salable_low=EXCLUDED.salable_low,
			salable_high=EXCLUDED.salable_high, status=EXCLUDED.status, is_sale=EXCLUDED.is_sale,
			sale_start=EXCLUDED.sale_start, sale_end=EXCLUDED.sale_end,
			count_down=EXCLUDED.count_down, server_ts=EXCLUDED.server_ts,
			priority=EXCLUDED.priority, target_interval_ms=EXCLUDED.target_interval_ms,
			replica_factor=EXCLUDED.replica_factor, enabled=EXCLUDED.enabled,
			updated_at=now()`,
		m.ID, m.Name, m.Flag, m.FlagText, m.Hot, m.WishCount, m.PriceLow, m.PriceHigh,
		m.SalableLow, m.SalableHigh, m.Status, m.IsSale, m.SaleStart, m.SaleEnd,
		m.CountDown, m.ServerTs, m.Priority, m.TargetIntervalMs, m.ReplicaFactor, m.Enabled)
	return err
}

func (p *PG) UpsertScreen(ctx context.Context, s model.Screen) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
		INSERT INTO screen_state (
			project_id, screen_id, name, start_time, flag, flag_text, clickable,
			sale_start, sale_end, server_ts, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
		ON CONFLICT (project_id, screen_id) DO UPDATE SET
			name=EXCLUDED.name, start_time=EXCLUDED.start_time, flag=EXCLUDED.flag,
			flag_text=EXCLUDED.flag_text, clickable=EXCLUDED.clickable,
			sale_start=EXCLUDED.sale_start, sale_end=EXCLUDED.sale_end,
			server_ts=EXCLUDED.server_ts, updated_at=now()`,
		s.ProjectID, s.ScreenID, s.Name, s.StartTime, s.Flag, s.FlagText,
		s.Clickable, s.SaleStart, s.SaleEnd, s.ServerTs)
	return err
}

func (p *PG) DeleteScreen(ctx context.Context, projectID, screenID int64) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `DELETE FROM screen_state WHERE project_id=$1 AND screen_id=$2`, projectID, screenID)
	return err
}

func (p *PG) UpsertSKU(ctx context.Context, s model.SKU) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
		INSERT INTO sku_state (
			project_id, screen_id, sku_id, flag, flag_text, clickable, price_fen, num, limit_num, descr, server_ts, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())
		ON CONFLICT (project_id, screen_id, sku_id) DO UPDATE SET
			flag=EXCLUDED.flag, flag_text=EXCLUDED.flag_text, clickable=EXCLUDED.clickable,
			price_fen=EXCLUDED.price_fen, num=EXCLUDED.num, limit_num=EXCLUDED.limit_num,
			descr=EXCLUDED.descr, server_ts=EXCLUDED.server_ts, updated_at=now()`,
		s.ProjectID, s.ScreenID, s.SkuID, s.Flag, s.FlagText, s.Clickable,
		s.PriceFen, s.Num, s.LimitNum, s.Desc, s.ServerTs)
	return err
}

func (p *PG) DeleteSKU(ctx context.Context, projectID, screenID, skuID int64) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `DELETE FROM sku_state WHERE project_id=$1 AND screen_id=$2 AND sku_id=$3`, projectID, screenID, skuID)
	return err
}

func (p *PG) InsertEvent(ctx context.Context, e model.Event) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	details, _ := json.Marshal(e.Details)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO state_events (
			project_id, screen_id, sku_id, event_type, severity, old_flag, new_flag,
			old_clickable, new_clickable, old_price_fen, price_fen, old_num, new_num, flag_text, server_ts, node_id, details, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,now())`,
		e.ProjectID, e.ScreenID, e.SkuID, e.EventType, e.Severity, e.OldFlag, e.NewFlag,
		e.OldClickable, e.NewClickable, e.OldPriceFen, e.PriceFen, e.OldNum, e.NewNum, e.FlagText, e.ServerTs, e.NodeID, details)
	return err
}

func (p *PG) UpsertCoverage(ctx context.Context, c model.PollCoverage) error {
	if c.ProjectID <= 0 || c.StartTs <= 0 || c.EndTs <= 0 {
		return nil
	}
	if c.EndTs < c.StartTs {
		c.EndTs = c.StartTs
	}
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
		INSERT INTO poll_coverage (project_id, start_ts, end_ts)
		VALUES ($1,$2,$3)
		ON CONFLICT (project_id, start_ts) DO UPDATE SET
			end_ts = GREATEST(poll_coverage.end_ts, EXCLUDED.end_ts)`,
		c.ProjectID, c.StartTs, c.EndTs)
	return err
}

func (p *PG) Coverage(ctx context.Context, projectID, sinceTs, untilTs int64) ([]model.PollCoverage, error) {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	rows, err := p.pool.Query(ctx, `
		SELECT project_id,start_ts,end_ts
		FROM poll_coverage
		WHERE project_id=$1
		  AND ($2::bigint <= 0 OR end_ts >= $2)
		  AND ($3::bigint <= 0 OR start_ts <= $3)
		ORDER BY start_ts ASC, end_ts ASC`,
		projectID, sinceTs, untilTs)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[model.PollCoverage])
	if out == nil {
		out = []model.PollCoverage{}
	}
	return out, err
}

func (p *PG) PruneBefore(ctx context.Context, cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `DELETE FROM poll_coverage WHERE end_ts < $1`, cutoffTs); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM state_events WHERE created_at < to_timestamp($1)`, cutoffTs)
	return err
}

func (p *PG) Snapshot(ctx context.Context) (model.Snapshot, error) {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	var snap model.Snapshot
	projectRows, err := p.pool.Query(ctx, `
		SELECT bili_project_id AS id,name,flag,flag_text,hot,wish_count,price_low,price_high,
		       salable_low,salable_high,status,is_sale,sale_start,sale_end,count_down,
		       server_ts,priority,target_interval_ms,replica_factor,enabled,updated_at
		FROM projects ORDER BY bili_project_id`)
	if err != nil {
		return snap, err
	}
	snap.Projects, err = pgx.CollectRows(projectRows, pgx.RowToStructByNameLax[model.Project])
	if err != nil {
		return snap, err
	}
	screenRows, err := p.pool.Query(ctx, `
		SELECT project_id,screen_id,name,start_time,flag,flag_text,clickable,sale_start,sale_end,server_ts,updated_at
		FROM screen_state ORDER BY project_id, screen_id`)
	if err != nil {
		return snap, err
	}
	snap.Screens, err = pgx.CollectRows(screenRows, pgx.RowToStructByNameLax[model.Screen])
	if err != nil {
		return snap, err
	}
	skuRows, err := p.pool.Query(ctx, `
		SELECT project_id,screen_id,sku_id,descr AS desc,flag,flag_text,clickable,price_fen,num,limit_num,server_ts,updated_at
		FROM sku_state ORDER BY project_id, screen_id, sku_id`)
	if err != nil {
		return snap, err
	}
	snap.SKUs, err = pgx.CollectRows(skuRows, pgx.RowToStructByNameLax[model.SKU])
	if err != nil {
		return snap, err
	}
	if err := p.applyLastEventTimes(ctx, &snap); err != nil {
		return snap, err
	}
	return snap, nil
}

func (p *PG) applyLastEventTimes(ctx context.Context, snap *model.Snapshot) error {
	projectTs, err := p.lastProjectEventTimes(ctx)
	if err != nil {
		return err
	}
	screenTs, err := p.lastScreenEventTimes(ctx)
	if err != nil {
		return err
	}
	skuTs, err := p.lastSKUEventTimes(ctx)
	if err != nil {
		return err
	}
	for i := range snap.Projects {
		if ts := projectTs[snap.Projects[i].ID]; ts > snap.Projects[i].LastEventTs {
			snap.Projects[i].LastEventTs = ts
		}
	}
	for i := range snap.Screens {
		k := screenKey{projectID: snap.Screens[i].ProjectID, screenID: snap.Screens[i].ScreenID}
		if ts := screenTs[k]; ts > snap.Screens[i].LastEventTs {
			snap.Screens[i].LastEventTs = ts
		}
	}
	for i := range snap.SKUs {
		k := skuKey{projectID: snap.SKUs[i].ProjectID, screenID: snap.SKUs[i].ScreenID, skuID: snap.SKUs[i].SkuID}
		if ts := skuTs[k]; ts > snap.SKUs[i].LastEventTs {
			snap.SKUs[i].LastEventTs = ts
		}
	}
	return nil
}

func (p *PG) lastProjectEventTimes(ctx context.Context) (map[int64]int64, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT project_id, MAX(COALESCE(NULLIF(server_ts, 0), EXTRACT(EPOCH FROM created_at)::bigint)) AS last_event_ts
		FROM state_events
		WHERE project_id > 0 AND severity <> 'ops'
		GROUP BY project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var projectID, ts int64
		if err := rows.Scan(&projectID, &ts); err != nil {
			return nil, err
		}
		out[projectID] = ts
	}
	return out, rows.Err()
}

func (p *PG) lastScreenEventTimes(ctx context.Context) (map[screenKey]int64, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT project_id, screen_id, MAX(COALESCE(NULLIF(server_ts, 0), EXTRACT(EPOCH FROM created_at)::bigint)) AS last_event_ts
		FROM state_events
		WHERE project_id > 0 AND screen_id > 0 AND severity <> 'ops'
		GROUP BY project_id, screen_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[screenKey]int64{}
	for rows.Next() {
		var projectID, screenID, ts int64
		if err := rows.Scan(&projectID, &screenID, &ts); err != nil {
			return nil, err
		}
		out[screenKey{projectID: projectID, screenID: screenID}] = ts
	}
	return out, rows.Err()
}

func (p *PG) lastSKUEventTimes(ctx context.Context) (map[skuKey]int64, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT project_id, screen_id, sku_id, MAX(COALESCE(NULLIF(server_ts, 0), EXTRACT(EPOCH FROM created_at)::bigint)) AS last_event_ts
		FROM state_events
		WHERE project_id > 0 AND screen_id > 0 AND sku_id > 0 AND severity <> 'ops'
		GROUP BY project_id, screen_id, sku_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[skuKey]int64{}
	for rows.Next() {
		var projectID, screenID, skuID, ts int64
		if err := rows.Scan(&projectID, &screenID, &skuID, &ts); err != nil {
			return nil, err
		}
		out[skuKey{projectID: projectID, screenID: screenID, skuID: skuID}] = ts
	}
	return out, rows.Err()
}

func (p *PG) Events(ctx context.Context, f EventFilter) ([]model.Event, error) {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	limit := eventLimit(f)
	clauses := []string{"true"}
	args := []any{}
	if f.ProjectID > 0 {
		args = append(args, f.ProjectID)
		clauses = append(clauses, fmt.Sprintf("project_id=$%d", len(args)))
	}
	if f.ScreenID > 0 {
		args = append(args, f.ScreenID)
		clauses = append(clauses, fmt.Sprintf("screen_id=$%d", len(args)))
	}
	if f.SkuID > 0 {
		args = append(args, f.SkuID)
		clauses = append(clauses, fmt.Sprintf("sku_id=$%d", len(args)))
	}
	if f.EventType != "" {
		args = append(args, f.EventType)
		clauses = append(clauses, fmt.Sprintf("event_type=$%d", len(args)))
	}
	if f.Severity != "" {
		args = append(args, f.Severity)
		clauses = append(clauses, fmt.Sprintf("severity=$%d", len(args)))
	}
	if f.SinceTs > 0 {
		args = append(args, f.SinceTs)
		clauses = append(clauses, fmt.Sprintf("created_at>=to_timestamp($%d)", len(args)))
	}
	if f.UntilTs > 0 {
		args = append(args, f.UntilTs)
		clauses = append(clauses, fmt.Sprintf("created_at<=to_timestamp($%d)", len(args)))
	}
	if !f.BeforeCreatedAt.IsZero() && f.BeforeID > 0 {
		args = append(args, f.BeforeCreatedAt, f.BeforeID)
		createdAtArg := len(args) - 1
		idArg := len(args)
		clauses = append(clauses, fmt.Sprintf("(created_at<$%d OR (created_at=$%d AND id<$%d))", createdAtArg, createdAtArg, idArg))
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT id,project_id,screen_id,sku_id,event_type,severity,old_flag,new_flag,
		       old_clickable,new_clickable,old_price_fen,price_fen,old_num,new_num,flag_text,server_ts,node_id,details,created_at
		FROM state_events WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		strings.Join(clauses, " AND "), len(args))
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	events, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[model.Event])
	if events == nil {
		events = []model.Event{}
	}
	return events, err
}

func (p *PG) Close() {
	p.pool.Close()
}

func (p *PG) opCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := p.opTimeout
	if timeout <= 0 {
		timeout = defaultPGOpTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (p *PG) migrate(ctx context.Context) error {
	ctx, cancel := p.opCtx(ctx)
	defer cancel()
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for i, entry := range entries {
		sql, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		version := i + 1
		var exists bool
		_ = p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists)
		if exists {
			continue
		}
		if _, err := p.pool.Exec(ctx, string(sql)); err != nil {
			return err
		}
		if _, err := p.pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT DO NOTHING`, version); err != nil {
			return err
		}
	}
	return nil
}

func quoteKV(v string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `'`, `\'`) + "'"
}
