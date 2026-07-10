package page

import (
	"sync"
	"testing"
)

// TestAppConfigLifecycle runs as ordered subtests because the singleton is
// package state: the unconfigured default must be observed before Configure.
// The previous state is restored on cleanup so the test is deterministic
// under -count=N (the singleton survives across runs in the same process).
func TestAppConfigLifecycle(t *testing.T) {
	prev := appConfig.Load()
	t.Cleanup(func() { appConfig.Store(prev) })

	t.Run("default when unconfigured", func(t *testing.T) {
		appConfig.Store(nil)
		c := AppConfig()
		if c.DefaultPageSize != 10 || c.DefaultPageNumber != 1 || c.MaxPageSize != FallbackMaxPageSize {
			t.Fatalf("unexpected default app config: %+v", c)
		}
	})

	t.Run("configure rejects invalid defaults", func(t *testing.T) {
		if err := Configure(Config{DefaultPageSize: 0, DefaultPageNumber: 1, MaxPageSize: 100}); err == nil {
			t.Fatal("expected error for default-pagesize 0")
		}
		if err := Configure(Config{DefaultPageSize: 10, DefaultPageNumber: 0, MaxPageSize: 100}); err == nil {
			t.Fatal("expected error for default-pagenumber 0")
		}
	})

	t.Run("configure then read", func(t *testing.T) {
		if err := Configure(Config{DefaultPageSize: 25, DefaultPageNumber: 1, MaxPageSize: 200}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		p := InitPaging(AppConfig(), -1, -1, 0)
		if p.PageSize != 25 {
			t.Fatalf("expected PageSize 25 from app config, got %d", p.PageSize)
		}
		if _, err := InitPaging(AppConfig(), 150, 1, 0).Paging(); err != nil {
			t.Fatalf("pageSize 150 should pass under MaxPageSize 200: %v", err)
		}
		if _, err := InitPaging(AppConfig(), 250, 1, 0).Paging(); err == nil {
			t.Fatal("expected ERR-PAGESIZE for pageSize 250 > MaxPageSize 200")
		}
	})

	t.Run("caller mutation has no effect", func(t *testing.T) {
		local := Config{DefaultPageSize: 30, DefaultPageNumber: 1, MaxPageSize: 300}
		if err := Configure(local); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		local.MaxPageSize = 1 // must not affect the stored copy
		if AppConfig().MaxPageSize != 300 {
			t.Fatalf("stored config was mutated by the caller: %+v", AppConfig())
		}
	})

	t.Run("concurrent readers", func(t *testing.T) {
		// Run with -race: lock-free reads while paging concurrently.
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := InitPaging(AppConfig(), 10, 1, 0).Paging(); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
	})
}
