// file: internal/runtime/behavior.go
package runtime

import (
	"crypto/sha512"
	"encoding/binary"
	"math"
	"time"
)

// SectionBehavior defines the timing and pattern used for a group of requests
// (e.g. a window of pages). It is meant to be deterministic per section and run,
// while still looking random enough from the outside.
type SectionBehavior struct {
	BaseDelay        time.Duration // base delay between requests (e.g. 300–1200ms)
	JitterFactor     float64       // relative jitter factor (e.g. 0.2–0.6 => ±20–60%)
	BurstEvery       int           // apply a larger pause every N requests
	BurstExtra       time.Duration // extra delay for burst pauses (e.g. 2–7s)
	FakeRequestProb  float64       // probability of issuing a fake "human-like" request
	PageShuffleWidth int           // window size used to slightly shuffle page order
}

// DeriveSectionBehavior builds a deterministic behavior profile for a given
// section, based on a strong run seed and some contextual inputs.
//
// The result is deterministic for the same (runSeed, username, sectionIndex, secret)
// tuple, but looks random across different runs and sections.
func DeriveSectionBehavior(
	runSeed []byte,
	username string,
	sectionIndex int,
	secret []byte,
) SectionBehavior {
	h := sha512.New()

	// Feed identity of this section. The order here must not change lightly,
	// otherwise behavior for existing runs will change.
	h.Write(runSeed)
	h.Write([]byte("|user:"))
	h.Write([]byte(username))
	h.Write([]byte("|section:"))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(sectionIndex))
	h.Write(buf[:])

	if len(secret) > 0 {
		h.Write([]byte("|secret:"))
		h.Write(secret)
	}

	sum := h.Sum(nil) // 64 bytes

	takeUint := func(offset int, max uint32) uint32 {
		if max == 0 {
			return 0
		}
		v := binary.BigEndian.Uint32(sum[offset : offset+4])
		return v % max
	}

	takeFloat01 := func(offset int) float64 {
		v := binary.BigEndian.Uint32(sum[offset : offset+4])
		return float64(v) / float64(math.MaxUint32)
	}

	// Base delay: 300–1200ms
	baseDelayMs := 300 + takeUint(0, 900) // 300–1199

	// Jitter factor: 0.2–0.6 (±20–60%)
	jitterFactor := 0.2 + takeFloat01(4)*0.4

	// Burst every: 15–59 requests
	burstEvery := 15 + int(takeUint(8, 45))

	// Burst extra delay: 2–7s
	burstExtraMs := 2000 + takeUint(12, 5000)

	// Fake request probability: 0–0.15
	fakeProb := takeFloat01(16) * 0.15

	// Page shuffle width: 1–4 (1 = no effective shuffle)
	shuffleWidth := 1 + int(takeUint(20, 4))

	return SectionBehavior{
		BaseDelay:        time.Duration(baseDelayMs) * time.Millisecond,
		JitterFactor:     jitterFactor,
		BurstEvery:       burstEvery,
		BurstExtra:       time.Duration(burstExtraMs) * time.Millisecond,
		FakeRequestProb:  fakeProb,
		PageShuffleWidth: shuffleWidth,
	}
}
