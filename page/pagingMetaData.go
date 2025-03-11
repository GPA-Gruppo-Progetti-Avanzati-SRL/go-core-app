package page

import (
	"errors"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
	"math"
)

const (
	QP_PageNumber = "pageNumber"
	QP_PageSize   = "pageSize"

	// DefaultPageSize   = 10
	// DefaultPageNumber = 1
	// MaxPageSize       = 15
)

var DefaultPageSize, DefaultPageNumber, MaxPageSize int

// Paging metadata
type Paging struct {
	PageSize    int   `json:"pageSize"`
	TotalItems  int64 `json:"totalItems"`
	TotalPages  int   `json:"totalPages"`
	CurrentPage int   `json:"currentPage"`
	HasNext     bool  `json:"hasNext"`
	HasPrev     bool  `json:"hasPrev"`
}

// Paging Request

// Configure default values
func config(pagingConfig *Config) {
	DefaultPageSize = pagingConfig.DefaultPageSize
	DefaultPageNumber = pagingConfig.DefaultPageNumber
	MaxPageSize = pagingConfig.MaxPageSize
}

// Initialize a new Paging Metadata.
// If pageSize is -1, set default value DefaultPageSize.
// If pageNumber is -1, return default value DefaultPageNumber.
func InitPaging(pagingConfig *Config, pageSize, pageNumber int, totalItems int64) *Paging {
	config(pagingConfig)

	var size, number int

	// If Page Size is nil, return default number of items.
	if pageSize == -1 {
		size = DefaultPageSize
	} else {
		size = pageSize
	}

	// If Page Number is nil, return default number of items.
	if pageNumber == -1 {
		number = DefaultPageNumber
	} else {
		number = pageNumber
	}

	// New Paging
	var p = &Paging{
		PageSize:    size,
		TotalItems:  totalItems,
		TotalPages:  0,
		CurrentPage: number,
		HasNext:     false,
		HasPrev:     false,
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
	errPS := validatorPageSize(p.PageSize)

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
	if p.TotalItems == 0 {
		p.setTotalPages(0)
	} else {
		p.setTotalPages(int(math.Ceil(float64(p.TotalItems) / float64(p.PageSize))))
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
	p.TotalItems = totalItems
	p.updatePagingMetaData()
}

// Increment TotalItems
func (p *Paging) IncTotalItems() {
	p.TotalItems++
	p.updatePagingMetaData()
}

// Decrement TotalItems
func (p *Paging) DecTotalItems() {
	p.TotalItems--
	p.updatePagingMetaData()
}

// Set TotalPages
func (p *Paging) setTotalPages(totalPages int) {
	p.TotalPages = totalPages
}

// Set PageSize
func (p *Paging) SetPageSize(pageSize int) *core.ApplicationError {
	err := validatorPageSize(pageSize)
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
	p.HasPrev = hasPrev
}

// Validator for PageSize
func validatorPageSize(param int) *core.ApplicationError {

	if param < -1 || param > MaxPageSize {
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
