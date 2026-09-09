package loadtest

import "testing"

func TestPercentile(t *testing.T) {
	v := []float64{100, 1, 4, 2, 3}
	if Percentile(v, .5) != 3 || Percentile(v, .99) != 100 || Percentile(nil, .99) != 0 {
		t.Fatal("incorrect nearest-rank percentile")
	}
	if v[0] != 100 {
		t.Fatal("mutated input")
	}
}
