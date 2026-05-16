package limiter

// QuotaConfig defines rate limit for a client+resource pair.
type QuotaConfig struct {
	Limit     int64 // max requests
	WindowSec int64 // window in seconds
}

// DefaultQuotas — fallback when no client-specific config is set.
var DefaultQuotas = map[string]QuotaConfig{
	"default": {Limit: 100, WindowSec: 60},
	"api":     {Limit: 1000, WindowSec: 60},
	"search":  {Limit: 50, WindowSec: 60},
	"upload":  {Limit: 10, WindowSec: 60},
	"auth":    {Limit: 5, WindowSec: 60},
}

// ClientOverrides — per-client quota overrides.
var ClientOverrides = map[string]map[string]QuotaConfig{
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
	if overrides, ok := ClientOverrides[clientID]; ok {
		if q, ok := overrides[resource]; ok {
			return q
		}
	}
	if q, ok := DefaultQuotas[resource]; ok {
		return q
	}
	return DefaultQuotas["default"]
}