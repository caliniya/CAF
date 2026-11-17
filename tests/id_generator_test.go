package Tests

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caliniya/caf/utils/id"
	"github.com/google/uuid"
)

func TestNewFormatVersionAndTimestamp(t *testing.T) {
	for _, kind := range []string{id.Memory, id.Chat, id.Node, id.Config, id.Workspace, id.QQAdapter, id.Request, id.Archive, id.Recollection, "custom_type1"} {
		before := time.Now().UnixMilli()
		value, err := id.New(kind)
		if err != nil {
			t.Fatal(err)
		}
		after := time.Now().UnixMilli()
		if len(value) != len(kind)+1+32 || !id.Valid(value, kind) || id.Valid(value, "other") {
			t.Fatalf("invalid format or type: %q", value)
		}
		parsed, err := uuid.Parse(strings.TrimPrefix(value, kind+"-"))
		if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 {
			t.Fatalf("ID is not a UUIDv7: %q / %v", value, err)
		}
		var milliseconds int64
		for _, b := range parsed[:6] {
			milliseconds = milliseconds<<8 | int64(b)
		}
		if milliseconds < before || milliseconds > after {
			t.Fatalf("UUIDv7 timestamp %d outside generation interval [%d, %d]", milliseconds, before, after)
		}
	}
}

func TestNewRejectsInvalidTypes(t *testing.T) {
	for _, kind := range []string{"", " memory", "memory ", "Memory", "1memory", "memory-note", "../memory", "a/b", "中文", strings.Repeat("a", 33)} {
		if value, err := id.New(kind); err == nil || value != "" {
			t.Fatalf("invalid type accepted: %q -> %q / %v", kind, value, err)
		}
	}
}

func TestValidRejectsOldFormatsAndInvalidUUIDs(t *testing.T) {
	value := id.MustNew(id.Memory)
	suffix := strings.TrimPrefix(value, id.Memory+"-")
	for _, invalid := range []string{
		suffix, "memory-" + uuid.MustParse(suffix).String(), "memory-" + strings.ToUpper(suffix),
		"memory-" + strings.Repeat("0", 32), "memory-" + suffix[:12] + "4" + suffix[13:],
		"memory-" + suffix[:16] + "0" + suffix[17:], "memory-" + suffix[:31], value + "0", value + "/..",
	} {
		if id.Valid(invalid, id.Memory) {
			t.Fatalf("invalid or legacy ID accepted: %q", invalid)
		}
	}
}

func TestNewOrdersSequentialIDs(t *testing.T) {
	previous := ""
	for range 1000 {
		current := id.MustNew(id.Memory)
		if previous >= current {
			t.Fatalf("sequential IDs are not increasing: %q >= %q", previous, current)
		}
		previous = current
	}
}

func TestNewConcurrentIDsDoNotCollide(t *testing.T) {
	var wg sync.WaitGroup
	values := make(chan string, 1000)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				value, err := id.New(id.Memory)
				if err != nil {
					t.Error(err)
					return
				}
				values <- value
			}
		}()
	}
	wg.Wait()
	close(values)
	seen := make(map[string]bool)
	for value := range values {
		if seen[value] || !id.Valid(value, id.Memory) {
			t.Fatalf("duplicate or invalid concurrent ID: %q", value)
		}
		seen[value] = true
	}
	if len(seen) != 1000 {
		t.Fatalf("generated %d IDs, want 1000", len(seen))
	}
}

type brokenRandomSource struct{ err error }

func (r brokenRandomSource) Read([]byte) (int, error) { return 0, r.err }

func TestNewReturnsRandomSourceErrorWithoutFallback(t *testing.T) {
	want := errors.New("random source unavailable")
	uuid.SetRand(brokenRandomSource{want})
	defer uuid.SetRand(nil)
	if value, err := id.New(id.Memory); value != "" || !errors.Is(err, want) {
		t.Fatalf("generation did not return the original error: %q / %v", value, err)
	}
	defer func() {
		if recover() == nil {
			t.Error("MustNew must panic when generation fails")
		}
	}()
	id.MustNew(id.Memory)
}
