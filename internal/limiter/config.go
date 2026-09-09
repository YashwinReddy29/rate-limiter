package limiter

// QuotaConfig defines rate limit for a client+resource pair.
type QuotaConfig struct {
	Limit     int64 // max requests
	WindowSec int64 // window in seconds
}

// defaultQuotas — fallback when no client-specific config is set.
var defaultQuotas = map[string]QuotaConfig{
	"default": {Limit: 100, WindowSec: 60},
	"api":     {Limit: 1000, WindowSec: 60},
	"search":  {Limit: 50, WindowSec: 60},
	"upload":  {Limit: 10, WindowSec: 60},
	"auth":    {Limit: 5, WindowSec: 60},
}

// clientOverrides — per-client quota overrides.
var clientOverrides = map[string]map[string]QuotaConfig{
	"premium_client": {
		"api":    {Limit: 10000, WindowSec: 60},
		"search": {Limit: 500, WindowSec: 60},
	},
	"free_client": {
		"api":    {Limit: 100, WindowSec: 60},
		"search": {Limit: 10, WindowSec: 60},
	},
}

func GetQuota(clientID, resource string) QuotaConfig {
	if overrides, ok := clientOverrides[clientID]; ok {
		if q, ok := overrides[resource]; ok {
			return q
		}
	}
	if q, ok := defaultQuotas[resource]; ok {
		return q
	}
	return defaultQuotas["default"]
}
