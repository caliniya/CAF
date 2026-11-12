package Tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	netapi "github.com/caliniya/caf/NetClient/NetAPI"
)

type timeTransport func(*http.Request) (*http.Response, error)

func (f timeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestTimeClientOnlineBeijingTime(t *testing.T) {
	serverUTC := time.Date(2026, 11, 10, 18, 30, 0, 123000000, time.UTC)
	client := netapi.NewTimeClient("")
	client.Client = &http.Client{Transport: timeTransport(func(request *http.Request) (*http.Response, error) {
		body, err := json.Marshal(map[string]any{
			"schemaVersion": 1,
			"utc":           serverUTC.Format(time.RFC3339Nano),
			"unixMs":        serverUTC.UnixMilli(),
			"source":        "cloudflare",
			"requestId":     "time-zone-test",
			"nonce":         request.URL.Query().Get("nonce"),
		})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}

	sample := client.Now(context.Background())
	if sample.Source != "cloudflare" || sample.ServerUTC == nil || !sample.ServerUTC.Equal(serverUTC) {
		t.Fatalf("server sample was not preserved: %+v", sample)
	}
	if _, offset := sample.Time.Zone(); offset != 8*60*60 {
		t.Fatalf("returned offset = %d, want UTC+08:00", offset)
	}
	if sample.Time.Format("2006-01-02 15:04") != "2026-11-11 02:30" {
		t.Fatalf("Beijing date did not cross midnight correctly: %v", sample.Time)
	}
	if _, offset := sample.ServerUTC.Zone(); offset != 0 {
		t.Fatalf("server original is no longer UTC: %v", sample.ServerUTC)
	}
	if delta := sample.Time.Sub(serverUTC) - time.Duration(sample.RoundTripMS*float64(time.Millisecond))/2; delta < -time.Nanosecond || delta > time.Nanosecond {
		t.Fatalf("timezone conversion changed the timestamp or network compensation: delta=%v", delta)
	}
	body, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	var serialized map[string]json.RawMessage
	if err := json.Unmarshal(body, &serialized); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(serialized["time"]), "+08:00\"") || !strings.HasSuffix(string(serialized["serverUtc"]), "Z\"") {
		t.Fatalf("JSON does not distinguish Beijing time from original UTC: %s", body)
	}
	if _, exists := serialized["utc"]; exists {
		t.Fatalf("obsolete UTC field remains in result: %s", body)
	}
}

func TestTimeClientLocalBeijingTime(t *testing.T) {
	client := netapi.NewTimeClient("invalid-endpoint")
	before := time.Now()
	sample := client.Now(context.Background())
	after := time.Now()
	if sample.Source != "local" || sample.FallbackReason == "" || sample.ServerUTC != nil {
		t.Fatalf("fallback source was not recorded: %+v", sample)
	}
	if _, offset := sample.Time.Zone(); offset != 8*60*60 {
		t.Fatalf("fallback offset = %d, want UTC+08:00", offset)
	}
	if sample.Time.Before(before) || sample.Time.After(after) {
		t.Fatalf("fallback changed the instant instead of its timezone: %v", sample.Time)
	}
}

func TestTimeSampleToStringIncludesTimeZone(t *testing.T) {
	instant := time.Date(2026, 11, 10, 18, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		zone *time.Location
		want string
	}{
		{"UTC", time.UTC, "时间:2026-11-10 18:30:00; 时区:+00:00"},
		{"Beijing", time.FixedZone("Asia/Shanghai", 8*60*60), "时间:2026-11-11 02:30:00; 时区:+08:00"},
		{"NegativeOffset", time.FixedZone("UTC-07", -7*60*60), "时间:2026-11-10 11:30:00; 时区:-07:00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sample := netapi.TimeSample{Time: instant.In(test.zone)}
			if got := sample.ToString(); got != test.want {
				t.Fatalf("ToString() = %q, want %q", got, test.want)
			}
		})
	}
}
