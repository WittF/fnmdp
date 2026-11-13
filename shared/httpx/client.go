package httpx

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

var (
	HelloChrome120 = utls.HelloChrome_120
	HelloChrome131 = utls.HelloChrome_131
	HelloChrome133 = utls.HelloChrome_133
)

type Persona struct {
	UA              string
	SecChUa         string
	SecChUaPlatform string
	ClientHello     utls.ClientHelloID
}

type Option func(*options)

type options struct {
	timeout time.Duration
}

func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}

func NewClient(p Persona, opts ...Option) (*http.Client, error) {
	cfg := options{timeout: 30 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	if !p.ClientHello.IsSet() {
		p.ClientHello = HelloChrome120
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport := newTransport(p)
	return &http.Client{
		Transport: &personaTransport{persona: p, base: transport},
		Jar:       jar,
		Timeout:   cfg.timeout,
	}, nil
}

func BootstrapBuvid(ctx context.Context, client *http.Client) error {
	if client == nil || client.Jar == nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bilibili.com/x/frontend/finger/spi", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("httpx: buvid bootstrap status %d", resp.StatusCode)
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Buvid3 string `json:"b_3"`
			Buvid4 string `json:"b_4"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if payload.Code != 0 {
		return fmt.Errorf("httpx: buvid bootstrap code %d", payload.Code)
	}
	var cookies []*http.Cookie
	if payload.Data.Buvid3 != "" {
		cookies = append(cookies, &http.Cookie{Name: "buvid3", Value: payload.Data.Buvid3, Path: "/"})
	}
	if payload.Data.Buvid4 != "" {
		cookies = append(cookies, &http.Cookie{Name: "buvid4", Value: payload.Data.Buvid4, Path: "/"})
	}
	if len(cookies) == 0 {
		return nil
	}
	for _, host := range []string{
		"https://api.bilibili.com/",
		"https://show.bilibili.com/",
		"https://mall.bilibili.com/",
	} {
		u, _ := url.Parse(host)
		client.Jar.SetCookies(u, cookies)
	}
	return nil
}

func newTransport(p Persona) http.RoundTripper {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	h2 := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialUTLS(ctx, dialer, network, addr, p, []string{"h2"})
		},
		TLSClientConfig: &tls.Config{NextProtos: []string{"h2"}},
		IdleConnTimeout: 90 * time.Second,
		PingTimeout:     15 * time.Second,
	}
	h1 := &http.Transport{
		Proxy:       http.ProxyFromEnvironment,
		DialContext: dialer.DialContext,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialUTLS(ctx, dialer, network, addr, p, []string{"http/1.1"})
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{NextProtos: []string{"http/1.1"}},
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	return &browserTransport{h2: h2, h1: h1}
}

func dialUTLS(ctx context.Context, dialer *net.Dialer, network, addr string, p Persona, nextProtos []string) (net.Conn, error) {
	raw, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		raw.Close()
		return nil, err
	}
	cfg := &utls.Config{
		ServerName: host,
		NextProtos: nextProtos,
	}
	conn := utls.UClient(raw, cfg, p.ClientHello)
	if err := conn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	return conn, nil
}

type personaTransport struct {
	persona Persona
	base    http.RoundTripper
}

type browserTransport struct {
	h2 http.RoundTripper
	h1 http.RoundTripper
}

func (t *browserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.h1.RoundTrip(req)
	}
	resp, err := t.h2.RoundTrip(req)
	if err == nil {
		return resp, nil
	}
	clone, ok := replayable(req)
	if !ok {
		return nil, err
	}
	return t.h1.RoundTrip(clone)
}

func (t *personaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := new(http.Request)
	*clone = *req
	clone.Header = req.Header.Clone()
	if t.persona.UA != "" {
		clone.Header.Set("User-Agent", t.persona.UA)
	}
	if t.persona.SecChUa != "" {
		clone.Header.Set("sec-ch-ua", t.persona.SecChUa)
	}
	if t.persona.SecChUaPlatform != "" {
		clone.Header.Set("sec-ch-ua-platform", t.persona.SecChUaPlatform)
	}
	clone.Header.Set("sec-ch-ua-mobile", "?0")
	clone.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	return t.base.RoundTrip(clone)
}

func replayable(req *http.Request) (*http.Request, bool) {
	clone := new(http.Request)
	*clone = *req
	clone.Header = req.Header.Clone()
	if req.Body == nil || req.Body == http.NoBody {
		return clone, true
	}
	if req.GetBody == nil {
		return nil, false
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}
	clone.Body = body
	return clone, true
}
