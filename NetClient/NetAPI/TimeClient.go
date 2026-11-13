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
	"sync"
	"time"
)

const DefaultTimeEndpoint = "https://caf-netapi-caliniya.pages.dev/api/time"

const defaultTimeTimeout = 10 * time.Second

const defaultSyncInterval = time.Minute

const defaultRetryInterval = 30 * time.Second

// 北京时间使用固定 UTC+08:00，不依赖本机时区或时区数据库。
var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// TimeSample 保存本次取时结果。Time 是北京时间下估计的接收时刻，ServerUTC 是服务器 UTC 原值。
// RoundTripMS 的一半用于网络补偿，不表示绝对时钟误差的上限。
type TimeSample struct {
	Time           time.Time  `json:"time"`
	Source         string     `json:"source"`
	Endpoint       string     `json:"endpoint"`
	ServerUTC      *time.Time `json:"serverUtc,omitempty"`
	RequestID      string     `json:"requestId,omitempty"`
	RoundTripMS    float64    `json:"roundTripMs"`
	FallbackReason string     `json:"fallbackReason,omitempty"`
	Cached         bool       `json:"cached"`
	SampleAgeMS    float64    `json:"sampleAgeMs"`
}

func (t *TimeSample) ToString() string {
	return "时间:" + t.Time.Format("2006-01-02 15:04:05") + "; 时区:" + t.Time.Format("-07:00")
}

// TimeClient 复用服务器取时基准，按单调时钟推进当前时间，可并发使用。
// 调用方应长期复用同一实例；开始使用后不得复制实例或并发修改配置。
type TimeClient struct {
	Endpoint string
	Client   *http.Client
	Timeout  time.Duration
	// SyncInterval 为成功取时后的校准间隔，非正值使用默认的 1 分钟。
	SyncInterval time.Duration
	// RetryInterval 为取时失败后的重试间隔，非正值使用默认的 30 秒。
	RetryInterval time.Duration

	mu         sync.Mutex
	cache      *timeCache
	refreshing chan struct{}
}

type timeCache struct {
	sample   TimeSample
	observed time.Time // 保留 time.Now() 的单调时钟部分，不转换时区或序列化。
	nextSync time.Time
}

// NewTimeClient 使用默认 Cloudflare 地址，或传入完整的自定义时间接口地址。
func NewTimeClient(endpoint string) *TimeClient {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultTimeEndpoint
	}
	return &TimeClient{
		Endpoint:      endpoint,
		Timeout:       defaultTimeTimeout,
		SyncInterval:  defaultSyncInterval,
		RetryInterval: defaultRetryInterval,
		Client: &http.Client{
			// 重定向不应悄悄改变取时结果所记录的时间来源。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Now 首次或校准到期时请求服务器，其余调用在本地推进已保存的时间基准。
// 取时失败后使用本机基准，等待 RetryInterval 再重试，避免离线时不断发请求。
// Cached、SampleAgeMS 和原始 ServerUTC / RequestID 用于区分新取时与推算时间。
func (c *TimeClient) Now(ctx context.Context) *TimeSample {
	return c.now(ctx, false)
}

// Sync 主动校准一次，忽略缓存期限；与已经进行中的校准共享同一请求。
func (c *TimeClient) Sync(ctx context.Context) *TimeSample {
	return c.now(ctx, true)
}

func (c *TimeClient) now(ctx context.Context, force bool) *TimeSample {
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

	c.mu.Lock()
	if !force && c.cache != nil && c.cache.sample.Endpoint == endpoint && time.Now().Before(c.cache.nextSync) {
		result := c.cachedNow()
		c.mu.Unlock()
		return result
	}
	if done := c.refreshing; done != nil {
		c.mu.Unlock()
		select {
		case <-done:
			c.mu.Lock()
			result := c.cachedNow()
			c.mu.Unlock()
			return result
		case <-ctx.Done():
			// 等待者取消不应覆盖正在同步的共享缓存。
			return localTimeSample(endpoint, ctx.Err())
		}
	}
	done := make(chan struct{})
	c.refreshing = done
	c.mu.Unlock()

	sample, err := c.fetch(ctx, endpoint)
	observed := time.Now()
	interval := c.SyncInterval
	if interval <= 0 {
		interval = defaultSyncInterval
	}
	if err != nil {
		sample = *localTimeSample(endpoint, err)
		interval = c.RetryInterval
		if interval <= 0 {
			interval = defaultRetryInterval
		}
	}
	c.mu.Lock()
	c.cache = &timeCache{sample: sample, observed: observed, nextSync: observed.Add(interval)}
	c.refreshing = nil
	close(done)
	c.mu.Unlock()
	// 返回值与内部缓存相互独立，调用者修改结果不会污染后续取时。
	return cloneTimeSample(sample)
}

// cachedNow 的调用方持有 mu。elapsed 使用单调时钟，不受系统校时影响。
func (c *TimeClient) cachedNow() *TimeSample {
	result := cloneTimeSample(c.cache.sample)
	elapsed := time.Since(c.cache.observed)
	result.Time = result.Time.Add(elapsed)
	result.Cached = true
	result.SampleAgeMS = float64(elapsed) / float64(time.Millisecond)
	return result
}

func cloneTimeSample(sample TimeSample) *TimeSample {
	if sample.ServerUTC != nil {
		serverUTC := *sample.ServerUTC
		sample.ServerUTC = &serverUTC
	}
	return &sample
}

func localTimeSample(endpoint string, err error) *TimeSample {
	return &TimeSample{
		Time:           time.Now().In(beijingLocation),
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
		Time:        serverUTC.Add(roundTrip / 2).In(beijingLocation),
		Source:      "cloudflare",
		Endpoint:    endpoint,
		ServerUTC:   &serverUTC,
		RequestID:   payload.RequestID,
		RoundTripMS: float64(roundTrip) / float64(time.Millisecond),
	}, nil
}
