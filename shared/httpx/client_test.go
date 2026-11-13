package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPersonaTransportOverridesHeaders(t *testing.T) {
	p := Persona{
		UA:              "ua",
		SecChUa:         `"Chromium";v="120"`,
		SecChUaPlatform: `"Windows"`,
	}
	rt := &personaTransport{
		persona: p,
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("User-Agent") != "ua" {
				t.Fatalf("ua = %q", req.Header.Get("User-Agent"))
			}
			if req.Header.Get("sec-ch-ua") != `"Chromium";v="120"` {
				t.Fatalf("sec-ch-ua = %q", req.Header.Get("sec-ch-ua"))
			}
			if req.Header.Get("sec-ch-ua-platform") != `"Windows"` {
				t.Fatalf("platform = %q", req.Header.Get("sec-ch-ua-platform"))
			}
			if req.Header.Get("sec-ch-ua-mobile") != "?0" || req.Header.Get("Accept-Language") == "" {
				t.Fatalf("browser headers = %+v", req.Header)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
	}
	req, _ := http.NewRequest(http.MethodGet, "https://show.bilibili.com/", nil)
	req.Header.Set("User-Agent", "old")
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("User-Agent") != "old" {
		t.Fatal("transport mutated caller request")
	}
}

func TestBootstrapBuvidSeedsCookieJar(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":0,"data":{"b_3":"b3","b_4":"b4"}}`)),
			}, nil
		}),
	}
	if err := BootstrapBuvid(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://mall.bilibili.com/")
	got := map[string]string{}
	for _, c := range jar.Cookies(u) {
		got[c.Name] = c.Value
	}
	if got["buvid3"] != "b3" || got["buvid4"] != "b4" {
		t.Fatalf("cookies = %+v", got)
	}
}

func TestBrowserTransportFallsBackWithReplayableBody(t *testing.T) {
	bt := &browserTransport{
		h2: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("h2 unavailable")
		}),
		h1: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != `{"itemsId":1}` {
				t.Fatalf("fallback body = %q", string(b))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
	}
	req, err := http.NewRequest(http.MethodPost, "https://mall.bilibili.com/", strings.NewReader(`{"itemsId":1}`))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(`{"itemsId":1}`)), nil
	}
	if _, err := bt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
}
