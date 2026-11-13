package getv2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const mallEndpoint = "https://mall.bilibili.com/mall-search-items/items_detail/info"

type MallClient struct {
	HTTP      HTTPDoer
	UserAgent string
	Referer   string
}

func NewMall(doer HTTPDoer) *MallClient {
	if doer == nil {
		doer = http.DefaultClient
	}
	return &MallClient{
		HTTP:      doer,
		UserAgent: defaultUA,
		Referer:   "https://mall.bilibili.com/",
	}
}

func (c *MallClient) ConfigureUserAgent(userAgent string) {
	if userAgent != "" {
		c.UserAgent = userAgent
	}
}

func (c *MallClient) Fetch(ctx context.Context, projectID int64) (*RawResponse, error) {
	return c.FetchByItemsID(ctx, projectID)
}

func (c *MallClient) FetchByItemsID(ctx context.Context, itemsID int64) (*RawResponse, error) {
	if itemsID <= 0 {
		return nil, &ProjectError{Errno: 0, Msg: "missing mall itemsId"}
	}
	body, err := json.Marshal(map[string]int64{"itemsId": itemsID})
	if err != nil {
		return nil, fmt.Errorf("mall: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mallEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mall: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Referer", c.Referer)
	req.Header.Set("Origin", "https://mall.bilibili.com")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mall: do request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeMall(resp.Body, resp.StatusCode)
	case http.StatusTooManyRequests:
		return nil, &RateError{StatusCode: resp.StatusCode, Source: "mall_http", Sentinel: ErrSoftLimited}
	case http.StatusPreconditionFailed:
		return nil, &RateError{
			StatusCode:   resp.StatusCode,
			Source:       "mall_http",
			Sentinel:     ErrHardBlocked,
			SecRequestID: resp.Header.Get("x-sec-request-id"),
		}
	default:
		return nil, fmt.Errorf("mall: unexpected status %d", resp.StatusCode)
	}
}

func decodeMall(r io.Reader, statusCode int) (*RawResponse, error) {
	var raw mallResponse
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("mall: decode body: %w", err)
	}
	switch raw.Code {
	case 0:
		if raw.Data == nil {
			return nil, &ProjectError{Errno: 0, Msg: "mall missing data"}
		}
		return &RawResponse{Errno: 0, Data: raw.Data.rawData()}, nil
	case -429, 429:
		return nil, &RateError{
			StatusCode: statusCode,
			BodyErrno:  raw.Code,
			Source:     "mall_body",
			Sentinel:   ErrSoftLimited,
		}
	case -412, 412:
		return nil, &RateError{
			StatusCode: statusCode,
			BodyErrno:  raw.Code,
			Source:     "mall_body",
			Sentinel:   ErrHardBlocked,
		}
	default:
		return nil, &ProjectError{Errno: raw.Code, Msg: raw.message()}
	}
}

type mallResponse struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Msg     string    `json:"msg"`
	Data    *mallData `json:"data"`
}

func (r mallResponse) message() string {
	if r.Message != "" {
		return r.Message
	}
	return r.Msg
}

type mallData struct {
	ProjectID        int64        `json:"projectId"`
	ID               int64        `json:"id"`
	Name             string       `json:"name"`
	ProjectName      string       `json:"projectName"`
	SaleFlagNumber   *int         `json:"saleFlagNumber"`
	SaleFlag         *RawSaleFlag `json:"saleFlag"`
	Status           *int         `json:"status"`
	IsSale           *int         `json:"isSale"`
	SaleStart        *int64       `json:"saleStart"`
	SaleEnd          *int64       `json:"saleEnd"`
	CurrentTime      *int64       `json:"currentTime"`
	CountDown        *int64       `json:"countDown"`
	PriceLow         *int         `json:"priceLow"`
	PriceHigh        *int         `json:"priceHigh"`
	SalablePriceLow  *int         `json:"salablePriceLow"`
	SalablePriceHigh *int         `json:"salablePriceHigh"`
	HotProject       *bool        `json:"hotProject"`
	WishInfo         *RawWishInfo `json:"wishInfo"`
	ScreenList       []RawScreen  `json:"screenList"`
}

func (d mallData) rawData() *RawData {
	id := d.ProjectID
	if id == 0 {
		id = d.ID
	}
	name := d.Name
	if name == "" {
		name = d.ProjectName
	}
	return &RawData{
		ID:               id,
		Name:             name,
		SaleFlagNumber:   d.SaleFlagNumber,
		SaleFlag:         rawProjectSaleFlag(d.SaleFlag),
		Status:           d.Status,
		IsSale:           d.IsSale,
		SaleStart:        d.SaleStart,
		SaleEnd:          d.SaleEnd,
		CurrentTime:      d.CurrentTime,
		CountDown:        d.CountDown,
		PriceLow:         d.PriceLow,
		PriceHigh:        d.PriceHigh,
		SalablePriceLow:  d.SalablePriceLow,
		SalablePriceHigh: d.SalablePriceHigh,
		HotProject:       d.HotProject,
		WishInfo:         d.WishInfo,
		ScreenList:       d.ScreenList,
	}
}

func rawProjectSaleFlag(flag *RawSaleFlag) *RawProjectSaleFlag {
	if flag == nil {
		return nil
	}
	return &RawProjectSaleFlag{Number: flag.Number, Text: flag.DisplayName}
}
