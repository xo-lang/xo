package check

import (
	"fmt"
	"strconv"
	"strings"
)

// GCConfig is the `gc` lines of a main module's xo.mod (core 4.2,
// decision 0065): the base GOGC percent, the floor of the heap goal, and
// the soft memory limit. -1 means not set; Percent is GCOff for
// `gc percent off`. The environment (GOGC, GOMEMLIMIT, XO_GC_MIN_HEAP)
// overrides each at run time.
type GCConfig struct {
	Percent     int64
	MinHeap     int64
	MemoryLimit int64
}

// GCOff is GCConfig.Percent for `gc percent off`.
const GCOff = -2

// NoGCConfig is a GCConfig with nothing set.
var NoGCConfig = GCConfig{Percent: -1, MinHeap: -1, MemoryLimit: -1}

// IsSet reports whether any `gc` line was given.
func (c GCConfig) IsSet() bool { return c != NoGCConfig }

// parseGCLine applies one `gc <key> <value>` line (fields without "gc").
func (c *GCConfig) parseGCLine(fields []string) error {
	if len(fields) != 2 {
		return fmt.Errorf("want `gc percent N|off`, `gc min_heap SIZE`, or `gc memory_limit SIZE`")
	}
	key, val := fields[0], fields[1]
	switch key {
	case "percent":
		if val == "off" {
			c.Percent = GCOff
			return nil
		}
		n, err := strconv.ParseInt(val, 10, 32)
		if err != nil || n < 1 {
			return fmt.Errorf("gc percent %q: want a positive integer or off", val)
		}
		c.Percent = n
	case "min_heap", "memory_limit":
		n, err := parseByteSize(val)
		if err != nil {
			return fmt.Errorf("gc %s: %v", key, err)
		}
		if key == "min_heap" {
			c.MinHeap = n
		} else {
			c.MemoryLimit = n
		}
	default:
		return fmt.Errorf("unknown gc setting %q (want percent, min_heap, or memory_limit)", key)
	}
	return nil
}

// parseByteSize parses a byte size: a number with an optional unit B, KiB,
// MiB, GiB, TiB (the GOMEMLIMIT spelling) or KB, MB, GB, TB (powers of
// 1000). std/rt gc.go parses XO_GC_MIN_HEAP the same way.
func parseByteSize(s string) (int64, error) {
	units := []struct {
		suf string
		n   int64
	}{{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40}, {"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1}}
	num, mul := s, int64(1)
	for _, u := range units {
		if strings.HasSuffix(s, u.suf) {
			num, mul = strings.TrimSuffix(s, u.suf), u.n
			break
		}
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 0 || n > (1<<62)/mul {
		return 0, fmt.Errorf("invalid byte size %q (want a number with B, KiB, MiB, GiB, or TiB)", s)
	}
	return n * mul, nil
}
