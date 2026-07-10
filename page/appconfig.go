package page

import (
	"sync/atomic"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
)

// appConfig holds the application-wide paging policy singleton. Unlike the
// removed package globals (rewritten on every InitPaging call), it is written
// once at boot via Configure and read lock-free afterwards (atomic pointer
// load): no per-request writes, so it cannot reintroduce cross-request races.
var appConfig atomic.Pointer[Config]

// defaultAppConfig is what AppConfig returns when Configure was never called.
var defaultAppConfig = Config{DefaultPageSize: 10, DefaultPageNumber: 1, MaxPageSize: FallbackMaxPageSize}

// Configure sets the application-wide paging policy singleton. Call it once at
// boot (e.g. while reading the app config), before serving traffic. The Config
// is copied, so later mutations by the caller have no effect. It returns an
// error if the defaults are invalid; MaxPageSize <= 0 is allowed and falls back
// to FallbackMaxPageSize at validation time.
func Configure(c Config) *core.ApplicationError {
	if c.DefaultPageSize <= 0 || c.DefaultPageNumber <= 0 {
		return core.TechnicalErrorWithCodeAndMessage("ERR-PAGECFG", "invalid paging config: default-pagesize and default-pagenumber must be > 0")
	}
	appConfig.Store(&c)
	return nil
}

// AppConfig returns the application-wide paging policy set via Configure, or a
// safe default (10/1/FallbackMaxPageSize) when Configure was never called.
// Typical endpoint usage: page.InitPaging(page.AppConfig(), req.PageSize, req.PageNumber, 0).
func AppConfig() *Config {
	if c := appConfig.Load(); c != nil {
		return c
	}
	return &defaultAppConfig
}
