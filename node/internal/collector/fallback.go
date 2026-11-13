package collector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/shared/getv2"
)

var (
	ErrNoSecondaryFetcher = errors.New("collector: no secondary fallback fetcher")
	ErrNoMallFetcher      = ErrNoSecondaryFetcher
	ErrPrimaryCoolingDown = errors.New("collector: primary cooling down and no secondary fallback available")
	ErrSecondaryThrottled = errors.New("collector: secondary fallback throttled")
)

type userAgentConfigurer interface {
	ConfigureUserAgent(string)
}

type SecondaryThrottledError struct {
	RetryAfter time.Duration
}

func (e *SecondaryThrottledError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%v: retry after %s", ErrSecondaryThrottled, e.RetryAfter.Round(time.Millisecond))
	}
	return ErrSecondaryThrottled.Error()
}

func (e *SecondaryThrottledError) Unwrap() error {
	return ErrSecondaryThrottled
}

type FallbackFetcher struct {
	Primary         Fetcher
	Secondary       Fetcher
	PrimarySource   string
	SecondarySource string
	PrimaryRate     *ratelimit.State
	SecondaryRate   *ratelimit.State

	SecondaryMinInterval time.Duration

	mu                   sync.Mutex
	lastSecondaryAttempt time.Time
}

func NewFallbackFetcher(primary Fetcher, primarySource string, secondary Fetcher, secondarySource string, primaryRate, secondaryRate *ratelimit.State) *FallbackFetcher {
	if primaryRate == nil {
		primaryRate = ratelimit.New(time.Second, 10*time.Minute)
	}
	if secondaryRate == nil {
		secondaryRate = ratelimit.New(time.Second, 10*time.Minute)
	}
	if primarySource == "" {
		primarySource = "primary"
	}
	if secondarySource == "" {
		secondarySource = "secondary"
	}
	return &FallbackFetcher{
		Primary:         primary,
		Secondary:       secondary,
		PrimarySource:   primarySource,
		SecondarySource: secondarySource,
		PrimaryRate:     primaryRate,
		SecondaryRate:   secondaryRate,
	}
}

func (f *FallbackFetcher) ConfigureRate(softBackoff, hardCooldown time.Duration) {
	if f.PrimaryRate != nil {
		f.PrimaryRate.Configure(softBackoff, hardCooldown)
	}
	if f.SecondaryRate != nil {
		f.SecondaryRate.Configure(softBackoff, hardCooldown)
	}
}

func (f *FallbackFetcher) ConfigureUserAgent(userAgent string) {
	if userAgent == "" {
		return
	}
	if c, ok := f.Primary.(userAgentConfigurer); ok {
		c.ConfigureUserAgent(userAgent)
	}
	if c, ok := f.Secondary.(userAgentConfigurer); ok {
		c.ConfigureUserAgent(userAgent)
	}
}

func (f *FallbackFetcher) SetSecondaryMinInterval(d time.Duration) {
	if d < 0 {
		d = 0
	}
	f.mu.Lock()
	f.SecondaryMinInterval = d
	f.mu.Unlock()
}

func (f *FallbackFetcher) Fetch(ctx context.Context, projectID int64) (*getv2.RawResponse, error) {
	raw, _, err := f.FetchWithSource(ctx, projectID)
	return raw, err
}

func (f *FallbackFetcher) FetchWithSource(ctx context.Context, projectID int64) (*getv2.RawResponse, string, error) {
	if f.Primary == nil {
		return nil, "", errors.New("collector: missing primary fetcher")
	}
	if f.PrimaryRate != nil {
		if until := f.PrimaryRate.BlockedUntil(time.Now()); !until.IsZero() {
			raw, err := f.fetchSecondary(ctx, projectID)
			if err == nil {
				return raw, f.SecondarySource, nil
			}
			if errors.Is(err, ErrNoSecondaryFetcher) {
				return nil, "", ErrPrimaryCoolingDown
			}
			return nil, "", err
		}
		if err := f.PrimaryRate.Wait(ctx); err != nil {
			return nil, "", err
		}
		f.PrimaryRate.Begin()
	}
	raw, err := f.Primary.Fetch(ctx, projectID)
	if f.PrimaryRate != nil {
		f.PrimaryRate.End()
	}
	now := time.Now()
	if err == nil {
		if f.PrimaryRate != nil {
			f.PrimaryRate.RecordSuccess(now)
		}
		normalizeProjectID(raw, projectID)
		return raw, f.PrimarySource, nil
	}
	recordRate(f.PrimaryRate, err, now)
	if !shouldFallback(f.PrimarySource, err) {
		return nil, "", err
	}
	secondaryRaw, secondaryErr := f.fetchSecondary(ctx, projectID)
	if secondaryErr == nil {
		return secondaryRaw, f.SecondarySource, nil
	}
	if errors.Is(secondaryErr, ErrNoSecondaryFetcher) {
		return nil, "", err
	}
	return nil, "", fmt.Errorf("collector fallback: primary: %w; secondary: %w", err, secondaryErr)
}

func (f *FallbackFetcher) fetchSecondary(ctx context.Context, projectID int64) (*getv2.RawResponse, error) {
	if f.Secondary == nil {
		return nil, ErrNoSecondaryFetcher
	}
	if err := f.reserveSecondaryAttempt(); err != nil {
		return nil, err
	}
	if f.SecondaryRate != nil {
		if err := f.SecondaryRate.Wait(ctx); err != nil {
			return nil, err
		}
		f.SecondaryRate.Begin()
	}
	raw, err := f.Secondary.Fetch(ctx, projectID)
	if f.SecondaryRate != nil {
		f.SecondaryRate.End()
	}
	now := time.Now()
	if err != nil {
		recordRate(f.SecondaryRate, err, now)
		return nil, err
	}
	if f.SecondaryRate != nil {
		f.SecondaryRate.RecordSuccess(now)
	}
	normalizeProjectID(raw, projectID)
	return raw, nil
}

func (f *FallbackFetcher) reserveSecondaryAttempt() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SecondaryMinInterval <= 0 {
		return nil
	}
	now := time.Now()
	next := f.lastSecondaryAttempt.Add(f.SecondaryMinInterval)
	if !f.lastSecondaryAttempt.IsZero() && now.Before(next) {
		return &SecondaryThrottledError{RetryAfter: next.Sub(now)}
	}
	f.lastSecondaryAttempt = now
	return nil
}

func shouldFallback(primarySource string, err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, getv2.ErrProjectError) {
		return primarySource == "mall"
	}
	return true
}

func recordRate(rate *ratelimit.State, err error, now time.Time) {
	if rate == nil {
		return
	}
	switch {
	case errors.Is(err, getv2.ErrSoftLimited):
		rate.RecordSoft(now)
	case errors.Is(err, getv2.ErrHardBlocked):
		rate.RecordHard(now)
	}
}

func normalizeProjectID(raw *getv2.RawResponse, projectID int64) {
	if raw == nil || raw.Data == nil || raw.Data.ID != 0 {
		return
	}
	raw.Data.ID = projectID
}
