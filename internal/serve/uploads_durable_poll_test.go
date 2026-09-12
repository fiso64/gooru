package serve

import (
	"testing"
	"time"
)

func TestNextBackgroundUploadWaitPollInterval(t *testing.T) {
	tests := []struct {
		current time.Duration
		want    time.Duration
	}{
		{25 * time.Millisecond, 50 * time.Millisecond},
		{50 * time.Millisecond, 100 * time.Millisecond},
		{100 * time.Millisecond, 200 * time.Millisecond},
		{200 * time.Millisecond, 250 * time.Millisecond},
		{250 * time.Millisecond, 250 * time.Millisecond},
	}
	for _, test := range tests {
		if got := nextBackgroundUploadWaitPollInterval(test.current); got != test.want {
			t.Fatalf("next interval after %v = %v, want %v", test.current, got, test.want)
		}
	}
}
