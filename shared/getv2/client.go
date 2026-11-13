package getv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Sentinel errors classifying the two-tier rate control in front of getV2
// (docs/bilibili-getv2-monitoring.md §4). Use errors.Is to test. This layer classifies only and never
// retries — backoff/retry discipline belongs to the collector node.
var (
	// ErrSoftLimited is HTTP 429: OpenResty edge soft limit. Retriable after
	// backoff, but unchecked retries escalate to ErrHardBlocked.
	ErrSoftLimited = errors.New("getv2: soft limited (429)")
	// ErrHardBlocked is HTTP 412: Gaia security hard block, IP-scoped for
	// ~8–9 minutes. Stop polling from this egress; do not probe.
	ErrHardBlocked = errors.New("getv2: hard blocked (412)")
	// ErrProjectError is a non-rate-control getV2 body errno, for example an
	// unknown or unavailable project. It is not retried by the client.
	ErrProjectError = errors.New("getv2: project error")
)

const (
	endpoint = "https://show.bilibili.com/api/ticket/project/getV2"
	// defaultUA is the last-resort UA for tools that do not inject a node persona.
	defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	defaultReferer = "https://show.bilibili.com/"
)

// RateError carries diagnostics for a rate-control classification. The Gaia
// layer stamps an x-sec-request-id response header on 412s; we capture it for
// later correlation. Unwraps to the matching sentinel for errors.Is.
type RateError struct {
	StatusCode   int
	BodyErrno    int
	Source       string
	Sentinel     error  // ErrSoftLimited or ErrHardBlocked
	SecRequestID string // x-sec-request-id, present on 412
}

func (e *RateError) Error() string {
	src := e.Source
	if src == "" {
		src = "http"
	}
	prefix := e.Sentinel.Error()
	if e.BodyErrno != 0 {
		prefix = fmt.Sprintf("%s [body_errno=%d source=%s]", prefix, e.BodyErrno, src)
	}
	if e.SecRequestID != "" {
		return fmt.Sprintf("%s [x-sec-request-id=%s]", prefix, e.SecRequestID)
	}
	return prefix
}

func (e *RateError) Unwrap() error { return e.Sentinel }

// ProjectError carries a non-zero getV2 body errno that is not rate control.
type ProjectError struct {
	Errno int
	Msg   string
}

func (e *ProjectError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("%v [errno=%d msg=%q]", ErrProjectError, e.Errno, e.Msg)
	}
	return fmt.Sprintf("%v [errno=%d]", ErrProjectError, e.Errno)
}

func (e *ProjectError) Unwrap() error { return ErrProjectError }

// HTTPDoer is the minimal http.Client surface, injectable for tests.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client fetches raw getV2 responses. The zero value is not usable; use New.
type Client struct {
	HTTP      HTTPDoer
	UserAgent string
	Referer   string
}

// New returns a Client. A nil doer falls back to http.DefaultClient.
func New(doer HTTPDoer) *Client {
	if doer == nil {
		doer = http.DefaultClient
	}
	return &Client{HTTP: doer, UserAgent: defaultUA, Referer: defaultReferer}
}

func (c *Client) ConfigureUserAgent(userAgent string) {
	if userAgent != "" {
		c.UserAgent = userAgent
	}
}

// Fetch performs a single getV2 request for projectID and classifies the
// result. 200 → parsed *RawResponse; 429 → ErrSoftLimited; 412 → ErrHardBlocked
// (both wrapped in *RateError); any other status → a generic error. It does not
// retry — that is the caller's responsibility.
func (c *Client) Fetch(ctx context.Context, projectID int64) (*RawResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("getv2: build request: %w", err)
	}
	q := req.URL.Query()
	q.Set("id", fmt.Sprint(projectID))
	q.Set("project_id", fmt.Sprint(projectID))
	q.Set("requestSource", "neul-next")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Referer", c.Referer)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getv2: do request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("getv2: read body: %w", err)
		}
		var raw RawResponse
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("getv2: decode body: %w", err)
		}
		switch raw.Errno {
		case 0:
			if raw.Data == nil {
				return nil, &ProjectError{Errno: 0, Msg: "missing data"}
			}
			return &raw, nil
		case -429:
			return nil, &RateError{
				StatusCode: http.StatusOK,
				BodyErrno:  raw.Errno,
				Source:     "body",
				Sentinel:   ErrSoftLimited,
			}
		case -412:
			return nil, &RateError{
				StatusCode: http.StatusOK,
				BodyErrno:  raw.Errno,
				Source:     "body",
				Sentinel:   ErrHardBlocked,
			}
		default:
			return nil, &ProjectError{Errno: raw.Errno, Msg: raw.Message}
		}
	case http.StatusTooManyRequests: // 429
		return nil, &RateError{StatusCode: resp.StatusCode, Source: "http", Sentinel: ErrSoftLimited}
	case http.StatusPreconditionFailed: // 412
		return nil, &RateError{
			StatusCode:   resp.StatusCode,
			Source:       "http",
			Sentinel:     ErrHardBlocked,
			SecRequestID: resp.Header.Get("x-sec-request-id"),
		}
	default:
		return nil, fmt.Errorf("getv2: unexpected status %d", resp.StatusCode)
	}
}
