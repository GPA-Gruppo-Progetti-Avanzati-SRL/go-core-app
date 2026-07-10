package page

import (
	"errors"
	"math"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
)

const (
	QP_PageNumber = "pageNumber"
	QP_PageSize   = "pageSize"

	// FallbackMaxPageSize is the application-level safety cap applied when no
	// per-instance limit is available (Config.MaxPageSize <= 0, or a Paging not
	// built via InitPaging). Being a const it is immutable and therefore safe
	// under concurrency by construction. It protects the database from abnormal
	// page sizes even on a misconfigured service. Note the deliberate exception:
	// pageSize == 0 means "all items" and bypasses any cap (see Paging()).
	FallbackMaxPageSize = 100
)

// Paging metadata.
//
// A Paging is not safe for concurrent use: it belongs to a single request.
// It must be built via InitPaging — do not reconstruct it from the wire
// (JSON/headers): the unexported per-instance limit is not serialized, so a
// rebuilt Paging falls back to FallbackMaxPageSize.
type Paging struct {
	PageSize    int   `json:"pageSize"`
	TotalCount  int64 `json:"totalCount"`
	TotalPages  int   `json:"totalPages"`
	CurrentPage int   `json:"currentPage"`
	HasNext     bool  `json:"hasNext"`
	HasPrevious bool  `json:"hasPrevious"`

	// maxPageSize is the per-instance upper bound for PageSize, captured from
	// the Config passed to InitPaging. Keeping it per-instance (instead of the
	// former package-level global rewritten on every InitPaging call) is what
	// makes validation deterministic under concurrency: requests configured
	// with different limits can no longer interfere with each other.
	maxPageSize int
}

// InitPaging initializes a new Paging metadata. It is a pure function: it only
// reads the passed Config and writes the returned struct — no shared state.
// If pageSize is -1, the default value pagingConfig.DefaultPageSize is used.
// If pageNumber is -1, the default value pagingConfig.DefaultPageNumber is used.
func InitPaging(pagingConfig *Config, pageSize, pageNumber int, totalItems int64) *Paging {
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

// Validator for PageSize.
// Validates against the per-instance limit captured at InitPaging time; when no
// per-instance limit is set (Config.MaxPageSize <= 0, or a Paging not built via
// InitPaging) the immutable FallbackMaxPageSize applies, so the upper bound is
// never unbounded. param == 0 ("all items") deliberately bypasses the cap.
func (p *Paging) validatorPageSize(param int) *core.ApplicationError {
	if param < -1 {
		return core.BusinessErrorWithCodeAndMessage("ERR-PAGESIZE", "invalid page size")
	}

	max := p.maxPageSize
	if max <= 0 {
		max = FallbackMaxPageSize
	}
	if param > max {
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
