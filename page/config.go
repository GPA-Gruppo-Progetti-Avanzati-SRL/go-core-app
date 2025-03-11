package page

type Config struct {
	DefaultPageSize   int `yaml:"default-pagesize" mapstructure:"default-pagesize" json:"default-pagesize"`
	DefaultPageNumber int `yaml:"default-pagenumber" mapstructure:"default-pagenumber" json:"default-pagenumber"`
	MaxPageSize       int `yaml:"max-pagesize" mapstructure:"max-pagesize" json:"max-pagesize"`
}
