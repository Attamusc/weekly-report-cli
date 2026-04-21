package retry

import (
	"crypto/rand"
	"math"
	"math/big"
	"time"
)

// CalculateBackoff returns an exponential backoff duration with jitter.
// Formula: baseMs * 2^attempt ± 25% jitter (cryptographic randomness).
func CalculateBackoff(attempt int, baseMs int) time.Duration {
	backoffMs := baseMs * int(math.Pow(2, float64(attempt)))

	// Add jitter (±25%)
	jitterMs := backoffMs / 4
	jitterBig, _ := rand.Int(rand.Reader, big.NewInt(int64(jitterMs*2+1)))
	jitter := int(jitterBig.Int64()) - jitterMs

	return time.Duration(backoffMs+jitter) * time.Millisecond
}
