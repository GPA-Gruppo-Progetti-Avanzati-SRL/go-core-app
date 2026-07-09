package page

import (
	"sync"
	"testing"
)

// TestInitPagingConcurrentDifferentConfigs reproduces the cross-request bug:
// one caller uses a small MaxPageSize while another lists with a page size that
// is valid only under the larger limit. Before the per-instance fix, the shared
// global MaxPageSize made the larger request intermittently fail validation with
// "invalid page size". Run with -race to also catch the data race on the global.
func TestInitPagingConcurrentDifferentConfigs(t *testing.T) {
	small := &Config{DefaultPageSize: 1, DefaultPageNumber: 1, MaxPageSize: 1}
	large := &Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: 100}

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(2)

		// Emulates GetOperazioneAttiva: only ever asks for a single item.
		go func() {
			defer wg.Done()
			p := InitPaging(small, 1, 1, 0)
			if _, err := p.Paging(); err != nil {
				t.Errorf("small paging failed: %v", err)
			}
		}()

		// Emulates the pubblicazioni list with the default page size of 10.
		go func() {
			defer wg.Done()
			p := InitPaging(large, 10, 1, 0)
			if _, err := p.Paging(); err != nil {
				t.Errorf("large paging failed: %v", err)
			}
		}()
	}
	wg.Wait()
}
