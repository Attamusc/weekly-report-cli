package retry

import (
	"testing"
	"time"
)

func TestCalculateBackoff_ExponentialIncrease(t *testing.T) {
	baseMs := 1000

	// Run multiple samples to account for jitter
	const samples = 20
	for attempt := 0; attempt < 4; attempt++ {
		var total time.Duration
		for i := 0; i < samples; i++ {
			total += CalculateBackoff(attempt, baseMs)
		}
		avg := total / samples

		// Expected: baseMs * 2^attempt (without jitter)
		expectedMs := baseMs * (1 << attempt)
		expected := time.Duration(expectedMs) * time.Millisecond

		// Allow ±30% tolerance (jitter is ±25%)
		low := time.Duration(float64(expected) * 0.70)
		high := time.Duration(float64(expected) * 1.30)

		if avg < low || avg > high {
			t.Errorf("attempt %d: avg backoff %v not in expected range [%v, %v]", attempt, avg, low, high)
		}

		// Verify exponential growth vs previous attempt
		if attempt > 0 {
			prevExpected := time.Duration(baseMs*(1<<(attempt-1))) * time.Millisecond
			if expected <= prevExpected {
				t.Errorf("attempt %d: expected %v should be greater than attempt %d expected %v", attempt, expected, attempt-1, prevExpected)
			}
		}
	}
}

func TestCalculateBackoff_NonNegative(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		d := CalculateBackoff(attempt, 1000)
		if d < 0 {
			t.Errorf("attempt %d: got negative duration %v", attempt, d)
		}
	}
}
