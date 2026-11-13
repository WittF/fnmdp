package getv2

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFetchClassifiesBodyErrnoSoftLimited(t *testing.T) {
	c := New(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errno":-429,"msg":"too frequent"}`), nil
	}))

	_, err := c.Fetch(context.Background(), 1001701)
	if !errors.Is(err, ErrSoftLimited) {
		t.Fatalf("err = %v, want ErrSoftLimited", err)
	}
	var re *RateError
	if !errors.As(err, &re) {
		t.Fatalf("err type = %T, want *RateError", err)
	}
	if re.StatusCode != http.StatusOK || re.BodyErrno != -429 || re.Source != "body" {
		t.Fatalf("rate error = %+v", re)
	}
}

func TestFetchClassifiesBodyErrnoHardBlocked(t *testing.T) {
	c := New(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errno":-412,"msg":"risk"}`), nil
	}))

	_, err := c.Fetch(context.Background(), 1001701)
	if !errors.Is(err, ErrHardBlocked) {
		t.Fatalf("err = %v, want ErrHardBlocked", err)
	}
	var re *RateError
	if !errors.As(err, &re) {
		t.Fatalf("err type = %T, want *RateError", err)
	}
	if re.StatusCode != http.StatusOK || re.BodyErrno != -412 || re.Source != "body" {
		t.Fatalf("rate error = %+v", re)
	}
}

func TestFetchClassifiesProjectError(t *testing.T) {
	c := New(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errno":12345,"msg":"project not found"}`), nil
	}))

	_, err := c.Fetch(context.Background(), 1001701)
	if !errors.Is(err, ErrProjectError) {
		t.Fatalf("err = %v, want ErrProjectError", err)
	}
	var pe *ProjectError
	if !errors.As(err, &pe) {
		t.Fatalf("err type = %T, want *ProjectError", err)
	}
	if pe.Errno != 12345 || pe.Msg != "project not found" {
		t.Fatalf("project error = %+v", pe)
	}
}

func TestFetchClassifiesMissingDataAsProjectError(t *testing.T) {
	c := New(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errno":0,"msg":"ok","data":null}`), nil
	}))

	_, err := c.Fetch(context.Background(), 1001701)
	if !errors.Is(err, ErrProjectError) {
		t.Fatalf("err = %v, want ErrProjectError", err)
	}
}

func TestFetchDecodesProjectSaleFlagString(t *testing.T) {
	c := New(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"errno": 0,
			"msg": "success",
			"data": {
				"id": 1001653,
				"name": "上海·BilibiliWorld 2026",
				"sale_flag_number": 4,
				"sale_flag": "已售罄",
				"current_time": 1781954099,
				"screen_list": [{
					"id": 1009926,
					"saleFlag": {"number": 4, "display_name": "已售罄"},
					"ticket_list": [{
						"id": 893303,
						"desc": "超级大会员尊享联票",
						"sale_flag": {"number": 4, "display_name": "已售罄"},
						"clickable": false
					}]
				}]
			}
		}`), nil
	}))

	raw, err := c.Fetch(context.Background(), 1001653)
	if err != nil {
		t.Fatalf("Fetch error = %v", err)
	}
	ps := Normalize(raw)
	if ps.Flag != 4 || ps.FlagText != "已售罄" || len(ps.Skus) != 1 {
		t.Fatalf("normalized project = %+v", ps)
	}
	if ps.Skus[0].Flag != 4 || ps.Skus[0].FlagText != "已售罄" {
		t.Fatalf("sku = %+v", ps.Skus[0])
	}
}

func TestFetchHTTPStatusStillWins(t *testing.T) {
	c := New(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := jsonResponse(http.StatusPreconditionFailed, `{"errno":0}`)
		resp.Header.Set("x-sec-request-id", "sec-1")
		return resp, nil
	}))

	_, err := c.Fetch(context.Background(), 1001701)
	if !errors.Is(err, ErrHardBlocked) {
		t.Fatalf("err = %v, want ErrHardBlocked", err)
	}
	var re *RateError
	if !errors.As(err, &re) {
		t.Fatalf("err type = %T, want *RateError", err)
	}
	if re.StatusCode != http.StatusPreconditionFailed || re.Source != "http" || re.SecRequestID != "sec-1" {
		t.Fatalf("rate error = %+v", re)
	}
}
