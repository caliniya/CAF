package NetAPI

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultTimeEndpoint = "https://caf-netapi-caliniya.pages.dev/api/time"

const defaultTimeTimeout = 10 * time.Second

// TimeSample 保存本次取时结果。UTC 是估计的接收时刻，ServerUTC 是服务器原值。
// RoundTripMS 的一半用于网络补偿，不表示绝对时钟误差的上限。
type TimeSample struct {
	UTC            time.Time  `json:"utc"`
	Source         string     `json:"source"`
	Endpoint       string     `json:"endpoint"`
	ServerUTC      *time.Time `json:"serverUtc,omitempty"`
	RequestID      string     `json:"requestId,omitempty"`
	RoundTripMS    float64    `json:"roundTripMs"`
	FallbackReason string     `json:"fallbackReason,omitempty"`
}

func (t *TimeSample) ToString() string {
	return "UTC:" + "时间"
}

// TimeClient 可并发取时。构造后不要在请求期间修改配置。
type TimeClient struct {
	Endpoint string
	Client   *http.Client
	Timeout  time.Duration
}

// NewTimeClient 使用默认 Cloudflare 地址，或传入完整的自定义时间接口地址。
func NewTimeClient(endpoint string) *TimeClient {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultTimeEndpoint
	}
	return &TimeClient{
		Endpoint: endpoint,
		Timeout:  defaultTimeTimeout,
		Client: &http.Client{
			// 重定向不应悄悄改变取时结果所记录的时间来源。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Now 优先在线取时；网络、协议、超时或取消错误均回退到本机 UTC。
// 调用者通过 Source 和 FallbackReason 判断是否使用了未经校验的本机时钟。
func (c *TimeClient) Now(ctx context.Context) TimeSample {
	if c == nil {
		c = NewTimeClient("")
	}
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultTimeEndpoint
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sample, err := c.fetch(ctx, endpoint)
	if err == nil {
		return sample
	}
	return TimeSample{
		UTC:            time.Now().UTC(),
		Source:         "local",
		Endpoint:       endpoint,
		FallbackReason: err.Error(),
	}
}

func (c *TimeClient) fetch(ctx context.Context, endpoint string) (TimeSample, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return TimeSample{}, fmt.Errorf("无效时间接口地址")
	}
	nonce := rand.Text()
	query := parsed.Query()
	query.Set("nonce", nonce)
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return TimeSample{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache, no-store")

	client := c.Client
	if client == nil {
		client = NewTimeClient(endpoint).Client
	}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return TimeSample{}, fmt.Errorf("在线取时失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return TimeSample{}, fmt.Errorf("时间接口返回 HTTP %d", response.StatusCode)
	}
	const maxResponseBytes = 16 * 1024
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return TimeSample{}, fmt.Errorf("读取时间响应失败：%w", err)
	}
	roundTrip := time.Since(started)
	if len(body) > maxResponseBytes {
		return TimeSample{}, fmt.Errorf("时间响应超过大小限制")
	}
	var payload struct {
		SchemaVersion int    `json:"schemaVersion"`
		UTC           string `json:"utc"`
		UnixMS        *int64 `json:"unixMs"`
		Source        string `json:"source"`
		RequestID     string `json:"requestId"`
		Nonce         string `json:"nonce"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return TimeSample{}, fmt.Errorf("解析时间响应失败：%w", err)
	}
	serverUTC, err := time.Parse(time.RFC3339Nano, payload.UTC)
	if err != nil || payload.SchemaVersion != 1 || payload.Source != "cloudflare" ||
		payload.Nonce != nonce || strings.TrimSpace(payload.RequestID) == "" ||
		payload.UnixMS == nil || *payload.UnixMS < 0 ||
		!serverUTC.Equal(time.UnixMilli(*payload.UnixMS)) || serverUTC.Year() > 9999 {
		return TimeSample{}, fmt.Errorf("时间响应校验失败")
	}
	serverUTC = serverUTC.UTC()
	return TimeSample{
		UTC:         serverUTC.Add(roundTrip / 2),
		Source:      "cloudflare",
		Endpoint:    endpoint,
		ServerUTC:   &serverUTC,
		RequestID:   payload.RequestID,
		RoundTripMS: float64(roundTrip) / float64(time.Millisecond),
	}, nil
}
