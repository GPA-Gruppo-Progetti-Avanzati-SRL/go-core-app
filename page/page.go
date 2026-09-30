package page

// Page struct holding slices of mixed type items
//
// Deprecated: paginare in memoria con PagingItems non ha caller nelle librerie; la paginazione
// si fa lato query con Paging.Paging() (offset) e il PageSize come limit.
type Page[T any] struct {
	Pages [][]T
}

// PagingItems returns the items of selectedPage (1-based), splitting responseList in pages of
// pageSize items.
//
// Deprecated: see Page. It panics with pageSize <= 0 or a selectedPage out of range, and p.Pages
// grows at every call.
func (p *Page[T]) PagingItems(pageSize, selectedPage int, responseList []T, totalItems int) []T {
	// One page, containing <pageSize> items
	onePage := []T{}

	// Scanning the list of all items
	for n, v := range responseList {
		onePage = append(onePage, v)

		// If the page is full or all items have been scanned,
		// append page to list of pages and create a new page
		if n%pageSize == pageSize-1 || n == int(totalItems)-1 {
			p.Pages = append(p.Pages, onePage)
			onePage = []T{}
		}
	}

	// The response list contains only items of the selected page
	return p.Pages[selectedPage-1]
}
