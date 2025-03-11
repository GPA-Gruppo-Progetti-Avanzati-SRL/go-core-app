package page

// Page struct holding slices of mixed type items
type Page[T any] struct {
	Pages [][]T
}

// Paging
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
