package getv2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestMallFetchMapsToRawResponse(t *testing.T) {
	c := NewMall(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "mall.bilibili.com" {
			t.Fatalf("request = %s %s", req.Method, req.URL.String())
		}
		var body map[string]int64
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["itemsId"] != 2001 {
			t.Fatalf("itemsId body = %+v", body)
		}
		return jsonResponse(http.StatusOK, `{
			"code": 0,
			"data": {
				"projectId": 1001653,
				"projectName": "Mall Project",
				"saleFlagNumber": 2,
				"saleFlag": {"number": 2, "display_name": "在售"},
				"status": 1,
				"isSale": 1,
				"saleStart": 1750100000,
				"currentTime": 1750100010,
				"salablePriceLow": 8800,
				"hotProject": true,
				"wishInfo": {"count": 99},
				"screenList": [{
					"id": 10,
					"name": "Day 1",
					"start_time": 1750200000,
					"saleFlag": {"number": 2, "display_name": "在售"},
					"clickable": true,
					"ticket_list": [{
						"id": 1,
						"desc": "VIP",
						"price": 8800,
						"num": 5,
						"static_limit": {"num": 8},
						"sale_flag": {"number": 2, "display_name": "在售"},
						"clickable": true
					}]
				}]
			}
		}`), nil
	}))

	raw, err := c.FetchByItemsID(context.Background(), 2001)
	if err != nil {
		t.Fatal(err)
	}
	ps := Normalize(raw)
	if ps.ProjectID != 1001653 || ps.Name != "Mall Project" || !ps.HotProject || ps.WishCount != 99 {
		t.Fatalf("project = %+v", ps)
	}
	if len(ps.Skus) != 1 {
		t.Fatalf("skus = %+v", ps.Skus)
	}
	sku := ps.Skus[0]
	if sku.ScreenID != 10 || sku.SkuID != 1 || sku.Num != 5 || sku.LimitNum != 8 || !sku.Clickable {
		t.Fatalf("sku = %+v", sku)
	}
}

func TestMallFetchClassifiesMissingItem(t *testing.T) {
	c := NewMall(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"code":71100001,"message":"商品不存在"}`), nil
	}))

	_, err := c.FetchByItemsID(context.Background(), 404)
	if !errors.Is(err, ErrProjectError) {
		t.Fatalf("err = %v, want ErrProjectError", err)
	}
	var pe *ProjectError
	if !errors.As(err, &pe) || pe.Errno != 71100001 {
		t.Fatalf("project error = %+v", pe)
	}
}

func TestMallFetchAliasUsesProjectIDAsItemsID(t *testing.T) {
	c := NewMall(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]int64
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["itemsId"] != 1001701 {
			t.Fatalf("itemsId body = %+v", body)
		}
		return jsonResponse(http.StatusOK, `{"code":0,"data":{"projectId":1001701,"projectName":"BML"}}`), nil
	}))

	raw, err := c.Fetch(context.Background(), 1001701)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Data == nil || raw.Data.ID != 1001701 {
		t.Fatalf("raw = %+v", raw)
	}
}
