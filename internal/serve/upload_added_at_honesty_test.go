package serve

import (
	"testing"
	"time"
)

func TestUploadAddedAtQueueStrategiesDoNotFabricateFutureTimes(t *testing.T) {
	observed := time.Date(2024, 1, 2, 3, 4, 5, 400_000_000, time.UTC)

	for _, tc := range []struct {
		name     string
		strategy string
		index    int
		total    int
	}{
		{name: "queue last item", strategy: "queue", index: 999, total: 1000},
		{name: "reverse queue first item", strategy: "reverse_queue", index: 0, total: 1000},
		{name: "modtime fallback", strategy: "modtime", index: 999, total: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUploadAddedAt(tc.strategy, time.Time{}, observed, time.Time{}, time.Time{}, tc.index, tc.total)
			if got.After(observed) {
				t.Fatalf("added_at=%v is after the observed queue time %v", got, observed)
			}
		})
	}
}
