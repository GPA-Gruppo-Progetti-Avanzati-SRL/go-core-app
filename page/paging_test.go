package page

import (
	"encoding/json"
	"testing"
)

// TestPageSizeOverPerInstanceMax: before the per-instance fix this outcome was
// race-dependent (the last InitPaging in the process set the global bound); it
// must now fail deterministically, every time.
func TestPageSizeOverPerInstanceMax(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 5, DefaultPageNumber: 1, MaxPageSize: 5}, 10, 1, 0)
	if _, err := p.Paging(); err == nil {
		t.Fatal("expected ERR-PAGESIZE for pageSize 10 > MaxPageSize 5")
	}
}

// TestFallbackCapOnHandBuiltPaging fixes the semantics for a Paging not built
// via InitPaging: the immutable FallbackMaxPageSize applies — never unbounded.
func TestFallbackCapOnHandBuiltPaging(t *testing.T) {
	over := &Paging{PageSize: 500, CurrentPage: 1}
	if _, err := over.Paging(); err == nil {
		t.Fatal("expected ERR-PAGESIZE for hand-built Paging with PageSize 500 > fallback cap")
	}
	within := &Paging{PageSize: 50, CurrentPage: 1}
	if _, err := within.Paging(); err != nil {
		t.Fatalf("unexpected error for hand-built Paging with PageSize 50: %v", err)
	}
}

// TestFallbackCapOnMissingMaxPageSize: a Config without max-pagesize (e.g. a
// forgotten YAML key) must be capped by FallbackMaxPageSize, not unbounded.
func TestFallbackCapOnMissingMaxPageSize(t *testing.T) {
	cfg := &Config{DefaultPageSize: 10, DefaultPageNumber: 1}
	if _, err := InitPaging(cfg, 50, 1, 0).Paging(); err != nil {
		t.Fatalf("pageSize 50 should pass under fallback cap: %v", err)
	}
	if _, err := InitPaging(cfg, 200, 1, 0).Paging(); err == nil {
		t.Fatal("expected ERR-PAGESIZE for pageSize 200 > fallback cap 100")
	}
}

// TestExplicitMaxPageSizeWinsOverFallback: an explicit Config.MaxPageSize above
// the fallback const must be honored.
func TestExplicitMaxPageSizeWinsOverFallback(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: 500}, 300, 1, 0)
	if _, err := p.Paging(); err != nil {
		t.Fatalf("pageSize 300 should pass under explicit MaxPageSize 500: %v", err)
	}
}

// TestSetPageSizeRespectsPerInstanceMax covers the non-InitPaging validation path.
func TestSetPageSizeRespectsPerInstanceMax(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: 20}, 10, 1, 0)
	if err := p.SetPageSize(25); err == nil {
		t.Fatal("expected ERR-PAGESIZE for SetPageSize 25 > MaxPageSize 20")
	}
	if err := p.SetPageSize(15); err != nil {
		t.Fatalf("unexpected error for SetPageSize 15: %v", err)
	}
}

// TestPageSizeZeroMeansAllItems: pageSize 0 keeps the "all items" contract and
// deliberately bypasses the cap (explicit user decision).
func TestPageSizeZeroMeansAllItems(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: 100}, 0, 1, 42)
	offset, err := p.Paging()
	if err != nil {
		t.Fatalf("unexpected error for pageSize 0: %v", err)
	}
	if offset != -1 {
		t.Fatalf("expected offset -1 (no paging), got %d", offset)
	}
	if p.TotalPages != 1 {
		t.Fatalf("expected TotalPages 1 for pageSize 0, got %d", p.TotalPages)
	}
}

// TestDefaultsResolvedFromPassedConfig: -1 sentinels resolve from the Config
// argument, not from any shared state.
func TestDefaultsResolvedFromPassedConfig(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 25, DefaultPageNumber: 3, MaxPageSize: 100}, -1, -1, 0)
	if p.PageSize != 25 {
		t.Fatalf("expected PageSize 25 from Config default, got %d", p.PageSize)
	}
	if p.CurrentPage != 3 {
		t.Fatalf("expected CurrentPage 3 from Config default, got %d", p.CurrentPage)
	}
}

// TestMisconfiguredDefaultOverMax: DefaultPageSize > MaxPageSize must fail
// deterministically at validation time (it was race-dependent before).
func TestMisconfiguredDefaultOverMax(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 50, DefaultPageNumber: 1, MaxPageSize: 10}, -1, -1, 0)
	if _, err := p.Paging(); err == nil {
		t.Fatal("expected ERR-PAGESIZE for default 50 > MaxPageSize 10")
	}
}

// TestByValueCopyCarriesBound: copying a Paging by value carries the unexported
// per-instance bound with it.
func TestByValueCopyCarriesBound(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: 20}, 10, 1, 0)
	q := *p
	if err := q.SetPageSize(25); err == nil {
		t.Fatal("expected the copied Paging to enforce the same bound")
	}
}

// TestJSONRoundTripLosesBound documents the expected behavior: a Paging rebuilt
// from JSON loses the unexported bound and falls back to FallbackMaxPageSize.
// Paging is not meant to be reconstructed from the wire.
func TestJSONRoundTripLosesBound(t *testing.T) {
	p := InitPaging(&Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: 500}, 300, 1, 0)
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var q Paging
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, appErr := q.Paging(); appErr == nil {
		t.Fatal("expected ERR-PAGESIZE: rebuilt Paging (PageSize 300) must fall back to the 100 cap")
	}
}
