package Tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	netapi "github.com/caliniya/caf/NetClient/NetAPI"
)

type timeTestTransport func(*http.Request) (*http.Response, error)

func (f timeTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func timeTestResponse(r *http.Request, instant time.Time, id string) *http.Response {
	body, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"utc":           instant.UTC().Format(time.RFC3339Nano),
		"unixMs":        instant.UnixMilli(),
		"source":        "cloudflare",
		"requestId":     id,
		"nonce":         r.URL.Query().Get("nonce"),
	})
	if err != nil {
		panic(err)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}
}

func TestTimeClientReusesSampleAndAdvancesTime(t *testing.T) {
	instant := time.Date(2026, 11, 13, 5, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	c := netapi.NewTimeClient("")
	c.Client.Transport = timeTestTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return timeTestResponse(r, instant, "original"), nil
	})
	first := c.Now(context.Background())
	if first.Cached || first.SampleAgeMS != 0 || first.Source != "cloudflare" {
		t.Fatalf("unexpected initial sample: %+v", first)
	}
	base := first.Time
	// 验证公开返回的基准年龄和时间推进，不修改客户端内部缓存。
	time.Sleep(5 * time.Millisecond)
	first.Time = time.Time{}
	*first.ServerUTC = time.Time{}
	first.RequestID = "changed-by-caller"
	for range 100 {
		sample := c.Now(context.Background())
		if !sample.Cached || sample.SampleAgeMS < 5 || sample.Source != "cloudflare" ||
			sample.RequestID != "original" || !sample.ServerUTC.Equal(instant) || sample.Time.Sub(base) < 5*time.Millisecond {
			t.Fatalf("cached clock or evidence was corrupted: %+v", sample)
		}
		if _, offset := sample.Time.Zone(); offset != 8*60*60 {
			t.Fatalf("cached time lost Beijing timezone: %v", sample.Time)
		}
		*sample.ServerUTC = time.Time{} // Cached results are also independent copies.
	}
	if calls.Load() != 1 {
		t.Fatalf("100 cached reads made %d requests, want 1 total", calls.Load())
	}
}

func TestTimeClientResyncsWhenDueAndOnDemand(t *testing.T) {
	instant := time.Date(2026, 11, 13, 5, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	c := netapi.NewTimeClient("")
	c.SyncInterval = 50 * time.Millisecond
	c.Client.Transport = timeTestTransport(func(r *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		return timeTestResponse(r, instant.Add(time.Duration(call)*time.Minute), fmt.Sprint(call)), nil
	})
	c.Now(context.Background())
	time.Sleep(c.SyncInterval + time.Millisecond)
	second := c.Now(context.Background())
	if second.Cached || calls.Load() != 2 || second.RequestID != "2" ||
		!second.ServerUTC.Equal(instant.Add(2*time.Minute)) {
		t.Fatalf("expired sample was not refreshed: %+v, calls=%d", second, calls.Load())
	}
	forced := c.Sync(context.Background())
	if forced.Cached || forced.RequestID != "3" || calls.Load() != 3 {
		t.Fatalf("explicit sync reused a fresh cache: %+v", forced)
	}
}

func TestTimeClientConcurrentCallsShareOneRequest(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	c := netapi.NewTimeClient("")
	c.Client.Transport = timeTestTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return timeTestResponse(r, time.Date(2026, 11, 13, 5, 0, 0, 0, time.UTC), "shared"), nil
	})
	var wg sync.WaitGroup
	results := make(chan *netapi.TimeSample, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- c.Now(context.Background())
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	for sample := range results {
		if sample.Source != "cloudflare" || sample.RequestID != "shared" {
			t.Fatalf("concurrent call lost shared result: %+v", sample)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent callers made %d requests", calls.Load())
	}
}

func TestTimeClientFailureBackoffAndRecovery(t *testing.T) {
	var calls atomic.Int32
	c := netapi.NewTimeClient("")
	if c.RetryInterval != 30*time.Second {
		t.Fatalf("default retry interval = %v, want 30s", c.RetryInterval)
	}
	c.RetryInterval = 50 * time.Millisecond
	c.Client.Transport = timeTestTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
		}
		return timeTestResponse(r, time.Date(2026, 11, 13, 5, 0, 0, 0, time.UTC), "recovered"), nil
	})
	first := c.Now(context.Background())
	second := c.Now(context.Background())
	if first.Source != "local" || first.FallbackReason == "" || !second.Cached ||
		second.Source != "local" || second.FallbackReason != first.FallbackReason || calls.Load() != 1 {
		t.Fatalf("failure caused immediate retry or lost provenance: %+v / %+v", first, second)
	}
	time.Sleep(c.RetryInterval + time.Millisecond)
	recovered := c.Now(context.Background())
	if calls.Load() != 2 || recovered.Source != "cloudflare" || recovered.FallbackReason != "" || recovered.Cached {
		t.Fatalf("recovery did not clear fallback: %+v", recovered)
	}
}

func TestTimeClientCancelledWaiterDoesNotCancelSharedSync(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	c := netapi.NewTimeClient("")
	c.Client.Transport = timeTestTransport(func(r *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return timeTestResponse(r, time.Date(2026, 11, 13, 5, 0, 0, 0, time.UTC), "shared"), nil
	})
	result := make(chan *netapi.TimeSample, 1)
	go func() { result <- c.Now(context.Background()) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := c.Now(ctx)
	if cancelled.Source != "local" || !strings.Contains(cancelled.FallbackReason, "context canceled") {
		t.Fatalf("cancelled waiter did not stop: %+v", cancelled)
	}
	close(release)
	if sample := <-result; sample.Source != "cloudflare" {
		t.Fatalf("waiter corrupted shared sync: %+v", sample)
	}
	if cached := c.Now(context.Background()); cached.Source != "cloudflare" || !cached.Cached {
		t.Fatalf("waiter fallback was saved as shared state: %+v", cached)
	}
}
