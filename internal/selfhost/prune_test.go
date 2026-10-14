package selfhost

import (
	"testing"
)

// TestSelfhostPruneXoTests runs the Xo tests of selfhost/prune, the age
// and size pruning of xo's caches (decision 0223), on the in memory file
// system and the virtual clock: stale and over the cap entries go, an
// entry in use stays, the stamp limits prunes, the environment's limits.
func TestSelfhostPruneXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	runXoTests(t, "prune")
}
