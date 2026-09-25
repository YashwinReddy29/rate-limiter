package loadtest

import (
	"math"
	"sort"
)

// Percentile uses the nearest-rank definition on sorted millisecond samples.
func Percentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	values := append([]float64(nil), samples...)
	sort.Float64s(values)
	i := int(math.Ceil(p*float64(len(values)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(values) {
		i = len(values) - 1
	}
	return values[i]
}
