package page

import (
	"errors"
	"math"
	"sync"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
)

const (
	QP_PageNumber = "pageNumber"
	QP_PageSize   = "pageSize"

	// DefaultPageSize   = 10
	// DefaultPageNumber = 1
	// MaxPageSize       = 15
)

// These package-level globals are kept only for backward compatibility. They are
// updated as a side effect of InitPaging and guarded by pagingConfigMu so concurrent
// InitPaging calls don't data-race on them. New logic must NOT rely on them: the
// effective limit lives per-instance on Paging.maxPageSize.
var (
	pagingConfigMu                                  sync.RWMutex
	DefaultPageSize, DefaultPageNumber, MaxPageSize int
)

// Paging metadata
type Paging struct {
	PageSize    int   `json:"pageSize"`
	TotalCount  int64 `json:"totalCount"`
	TotalPages  int   `json:"totalPages"`
	CurrentPage int   `json:"currentPage"`
	HasNext     bool  `json:"hasNext"`
	HasPrevious bool  `json:"hasPrevious"`

	// maxPageSize is the per-instance upper bound for PageSize, captured from the
	// Config passed to InitPaging. It is unexported (so it is never serialized) and
	// makes validation independent from the package-level MaxPageSize global, which
	// is shared across goroutines and caused cross-request "invalid page size"
	// errors when concurrent requests used different Config.MaxPageSize values.
	maxPageSize int
}

// Paging Request

// Configure default values.
// Kept only to update the exported package-level globals for backward compatibility
// with external code that may still read them. InitPaging no longer relies on these
// globals for its own logic (see below), so it is safe under concurrency.
func config(pagingConfig *Config) {
	pagingConfigMu.Lock()
	defer pagingConfigMu.Unlock()
	DefaultPageSize = pagingConfig.DefaultPageSize
	DefaultPageNumber = pagingConfig.DefaultPageNumber
	MaxPageSize = pagingConfig.MaxPageSize
}

// Initialize a new Paging Metadata.
// If pageSize is -1, set default value pagingConfig.DefaultPageSize.
// If pageNumber is -1, return default value pagingConfig.DefaultPageNumber.
func InitPaging(pagingConfig *Config, pageSize, pageNumber int, totalItems int64) *Paging {
	// Keep the exported globals in sync for backward compatibility only.
	config(pagingConfig)

	// Resolve defaults from the passed-in config, NOT from the shared globals,
	// so concurrent callers with different Config values do not interfere.
	size := pageSize
	if pageSize == -1 {
		size = pagingConfig.DefaultPageSize
	}

	number := pageNumber
	if pageNumber == -1 {
		number = pagingConfig.DefaultPageNumber
	}

	// New Paging
	var p = &Paging{
		PageSize:    size,
		TotalCount:  totalItems,
		TotalPages:  0,
		CurrentPage: number,
		HasNext:     false,
		HasPrevious: false,
		maxPageSize: pagingConfig.MaxPageSize,
	}

	// Update
	p.updatePagingMetaData()

	return p
}

// If pageSize is set to 0, do not apply paging (get all items) and returns -1.
// If pageSize and pageNumber are set to a correct value, applies paging and returns the OFFSET.
// Otherwise, returns error.
func (p *Paging) Paging() (int, *core.ApplicationError) {

	var pageSize, pageNumber int

	// Page Size
	errPS := p.validatorPageSize(p.PageSize)

	// If Page Size is uncorrect, error
	// If Page Size is 0, return all items.
	// Otherwise, apply paging.
	if errPS != nil {
		return 0, core.BusinessErrorWithError(errPS)

	} else if p.PageSize == 0 {
		return -1, nil

	} else {
		pageSize = p.PageSize
	}

	// Page Number
	pageNumber = p.CurrentPage
	errPN := p.SetCurrentPage(pageNumber)
	if errPN != nil {
		return 0, core.BusinessErrorWithError(errPN)
	}

	// Set offset and limit
	var offset int = (pageNumber - 1) * pageSize

	return offset, nil
}

// Updating Paging
func (p *Paging) updatePagingMetaData() {

	// If there are no items, TotalPages = 0. Otherwise, calculate TotalPages
	if p.TotalCount == 0 {
		p.setTotalPages(0)
	} else {
		p.setTotalPages(int(math.Ceil(float64(p.TotalCount) / float64(p.PageSize))))
	}

	// If pageSize = 0, there is 1 page
	if p.PageSize == 0 {
		p.TotalPages = 1
	}

	// If CurrentPage > 1, the current page has a previous page. Orherwise, it hasn't
	if p.CurrentPage > 1 && p.CurrentPage <= p.TotalPages+1 {
		p.setHasPrev(true)
	} else {
		p.setHasPrev(false)
	}

	// If CurrentPage = TotalPages, the current page has not a next page. Orherwise, it has
	if p.CurrentPage >= p.TotalPages || p.CurrentPage == 0 {
		p.setHasNext(false)
	} else {
		p.setHasNext(true)
	}
}

// Set TotalItems
func (p *Paging) SetTotalItems(totalItems int64) {
	p.TotalCount = totalItems
	p.updatePagingMetaData()
}

// Increment TotalItems
func (p *Paging) IncTotalItems() {
	p.TotalCount++
	p.updatePagingMetaData()
}

// Decrement TotalItems
func (p *Paging) DecTotalItems() {
	p.TotalCount--
	p.updatePagingMetaData()
}

// Set TotalPages
func (p *Paging) setTotalPages(totalPages int) {
	p.TotalPages = totalPages
}

// Set PageSize
func (p *Paging) SetPageSize(pageSize int) *core.ApplicationError {
	err := p.validatorPageSize(pageSize)
	if err != nil {
		return core.BusinessErrorWithError(err)
	}

	p.PageSize = pageSize
	p.updatePagingMetaData()

	return nil
}

// Set CurrentPage
func (p *Paging) SetCurrentPage(currentPage int) *core.ApplicationError {
	err := validatorPageNumber(currentPage)
	if err != nil {
		return core.BusinessErrorWithError(err)
	}

	p.CurrentPage = currentPage
	p.updatePagingMetaData()

	return nil
}

// Increment CurrentPage
func (p *Paging) IncCurrentPage() {
	if p.CurrentPage < 0 {
		panic(errors.New("invalid current page"))
	}
	p.CurrentPage++

	p.updatePagingMetaData()
}

// Decrement CurrentPage
func (p *Paging) DecCurrentPage() {
	if p.CurrentPage < 0 {
		panic(errors.New("invalid current page"))
	}
	p.CurrentPage--

	p.updatePagingMetaData()
}

// Set HasNext
func (p *Paging) setHasNext(hasNext bool) {
	p.HasNext = hasNext
}

// Set HasPrev
func (p *Paging) setHasPrev(hasPrev bool) {
	p.HasPrevious = hasPrev
}

// effectiveMaxPageSize returns the per-instance limit captured at InitPaging time.
// For a Paging not built via InitPaging (maxPageSize == 0) it falls back to the
// package-level MaxPageSize global to preserve the previous behavior.
func (p *Paging) effectiveMaxPageSize() int {
	if p.maxPageSize > 0 {
		return p.maxPageSize
	}
	pagingConfigMu.RLock()
	defer pagingConfigMu.RUnlock()
	return MaxPageSize
}

// Validator for PageSize.
// Validates against the per-instance limit rather than the shared global, so
// concurrent requests configured with different MaxPageSize cannot fail each other.
func (p *Paging) validatorPageSize(param int) *core.ApplicationError {

	if param < -1 || param > p.effectiveMaxPageSize() {
		return core.BusinessErrorWithCodeAndMessage("ERR-PAGESIZE", "invalid page size")
	}

	return nil
}

// Validator for PageNumber
func validatorPageNumber(param int) *core.ApplicationError {

	if param < 1 {
		return core.BusinessErrorWithCodeAndMessage("ERR-PAGENUMBER", "invalid page number")
	}

	return nil

}
