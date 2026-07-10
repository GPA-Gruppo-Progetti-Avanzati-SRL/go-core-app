package page

// Config holds the paging policy passed to InitPaging. It is read-only input:
// InitPaging captures its values per-instance, so a single Config shared across
// the whole application is safe as long as it is not mutated after boot.
type Config struct {
	DefaultPageSize   int `yaml:"default-pagesize" mapstructure:"default-pagesize" json:"default-pagesize"`
	DefaultPageNumber int `yaml:"default-pagenumber" mapstructure:"default-pagenumber" json:"default-pagenumber"`
	// MaxPageSize is the upper bound for a requested page size. If <= 0 the
	// immutable FallbackMaxPageSize (100) applies — never unbounded. Deliberate
	// exception: pageSize == 0 means "all items" and bypasses the cap; expose it
	// to clients knowingly.
	MaxPageSize int `yaml:"max-pagesize" mapstructure:"max-pagesize" json:"max-pagesize"`
}
