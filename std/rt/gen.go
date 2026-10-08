package rt

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
)

// Value generation for property tests (`test "x" for v: T`, core 9.2) and
// `testing.gen[T](seed)` (std.md 8.5). Builtin types are generated here;
// generated structs, enums, and newtypes have an XoGenAny method.

// GenRand is the random source of one generated value, with a depth limit
// so recursive types terminate.
//
// Every random choice goes through draw and is recorded, so a failing
// input can be shrunk by editing the recorded choices (smaller, fewer)
// and replaying the generator: zero choices give the simplest values.
type GenRand struct {
	r     *rand.Rand
	depth int
	// Small keeps numbers small (contract fuzzing: a generated Int bound
	// for a loop must not run for ages).
	Small  bool
	rec    []uint64
	replay []uint64
	pos    int
}

// NewGenRand returns a generator seeded with seed.
func NewGenRand(seed uint64) *GenRand {
	return &GenRand{r: rand.New(rand.NewPCG(seed, seed^0x2545f4914f6cdd1d))}
}

// replayGen returns a generator that makes the given choices (then zeros).
func replayGen(choices []uint64, small bool) *GenRand {
	return &GenRand{replay: append([]uint64{}, choices...), Small: small}
}

// draw returns a choice in [0, n), or any 64 bit value when n is 0.
func (g *GenRand) draw(n uint64) uint64 {
	var v uint64
	switch {
	case g.replay != nil:
		if g.pos < len(g.replay) {
			v = g.replay[g.pos]
		}
		g.pos++
		if n > 0 {
			v %= n
		}
	case n == 0:
		v = g.r.Uint64()
	default:
		v = g.r.Uint64N(n)
	}
	g.rec = append(g.rec, v)
	return v
}

// Intn returns a value in [0, n).
func (g *GenRand) Intn(n int) int {
	if n <= 1 {
		return 0
	}
	return int(g.draw(uint64(n)))
}

// Enter and Leave bracket a nested value.
func (g *GenRand) Enter() { g.depth++ }

// Leave ends a nested value.
func (g *GenRand) Leave() { g.depth-- }

// Deep reports whether nested values should stop growing.
func (g *GenRand) Deep() bool { return g.depth >= 4 }

// size picks a collection length: small, shrinking with depth.
func (g *GenRand) size() int {
	if g.Deep() {
		return 0
	}
	return g.Intn(6 - g.depth)
}

type generator interface {
	XoGenAny(g *GenRand) any
}

// Gen generates a value of type T.
func Gen[T any](g *GenRand) T {
	var z T
	switch any(z).(type) {
	case int64:
		return any(genInt(g)).(T)
	case int32:
		return any(int32(genInt(g) % (1 << 31))).(T)
	case int16:
		return any(int16(genInt(g) % (1 << 15))).(T)
	case int8:
		return any(int8(genInt(g) % (1 << 7))).(T)
	case uint64:
		return any(uint64(abs64(genInt(g)))).(T)
	case uint32:
		return any(uint32(abs64(genInt(g)))).(T)
	case uint16:
		return any(uint16(abs64(genInt(g)))).(T)
	case uint8:
		return any(uint8(abs64(genInt(g)))).(T)
	case float64:
		return any(genFloat(g)).(T)
	case float32:
		return any(float32(genFloat(g))).(T)
	case bool:
		return any(g.Intn(2) == 1).(T)
	case string:
		return any(genStr(g)).(T)
	case Bytes:
		return any(Bytes(genStr(g))).(T)
	case Rune:
		return any(Rune([]rune(alphabet)[g.Intn(len([]rune(alphabet)))])).(T)
	case Duration:
		return any(Duration(genInt(g)) * Duration(1e6)).(T)
	case Time:
		return any(Time(1767225600e9 + genInt(g)*1e9)).(T)
	case Unit:
		return z
	case generator:
		return any(z).(generator).XoGenAny(g).(T)
	}
	FaultHere("cannot generate values of type %s for a property test", reflect.TypeOf(&z).Elem())
	return z
}

const alphabet = "abcxyzABC019 _-éλ😀"

func abs64(n int64) int64 {
	if n < 0 {
		if n == math.MinInt64 {
			return math.MaxInt64
		}
		return -n
	}
	return n
}

// zigzag maps choices to values so that smaller choices are simpler
// values: 0, -1, 1, -2, 2, ...
func zigzag(v uint64) int64 { return int64(v>>1) ^ -int64(v&1) }

// genInt prefers small values and edges; choice 0 is 0.
func genInt(g *GenRand) int64 {
	if g.Small {
		if g.draw(5) < 4 {
			return zigzag(g.draw(2001))
		}
		return []int64{0, 1, -1}[g.Intn(3)]
	}
	switch c := g.draw(10); {
	case c < 7:
		return zigzag(g.draw(201))
	case c == 7:
		return []int64{0, 1, -1, math.MaxInt64, math.MinInt64}[g.Intn(5)]
	}
	return zigzag(g.draw(0))
}

// genFloat prefers small values; choice 0 is 0.0.
func genFloat(g *GenRand) float64 {
	switch c := g.draw(8); {
	case c < 6:
		return float64(zigzag(g.draw(2001))) / 8
	case c == 6:
		return []float64{0, -0.5, 1, 1e300, -1e-300}[g.Intn(5)]
	}
	return (float64(g.draw(1<<53))/(1<<53) - 0.5) * 1000
}

func genStr(g *GenRand) string {
	rs := []rune(alphabet)
	n := g.Intn(9)
	out := make([]rune, n)
	for i := range out {
		out[i] = rs[g.Intn(len(rs))]
	}
	return string(out)
}

// XoGenAny generates a list.
func (List[T]) XoGenAny(g *GenRand) any {
	g.Enter()
	defer g.Leave()
	n := g.size()
	xs := make([]T, n)
	for i := range xs {
		xs[i] = Gen[T](g)
	}
	return ListOf(xs...)
}

// XoGenAny generates a set.
func (Set[T]) XoGenAny(g *GenRand) any {
	l := List[T]{}.XoGenAny(g).(List[T])
	return l.M_to_set(nil)
}

// XoGenAny generates a map.
func (Map[K, V]) XoGenAny(g *GenRand) any {
	g.Enter()
	defer g.Leave()
	var m Map[K, V]
	for i, n := 0, g.size(); i < n; i++ {
		m.M_put(nil, Gen[K](g), Gen[V](g))
	}
	return m
}

// XoGenAny generates an optional.
func (Option[T]) XoGenAny(g *GenRand) any {
	if g.Deep() || g.Intn(4) == 0 {
		return Option[T]{}
	}
	g.Enter()
	defer g.Leave()
	return Some(Gen[T](g))
}

// XoGenAny generates a Result.
func (Result[T, E]) XoGenAny(g *GenRand) any {
	g.Enter()
	defer g.Leave()
	if g.Intn(3) == 0 {
		return Err[T](Gen[E](g))
	}
	return Ok[T, E](Gen[T](g))
}

// XoGenAny generates a tuple.
func (Tuple2[A, B]) XoGenAny(g *GenRand) any {
	return Tuple2[A, B]{Gen[A](g), Gen[B](g)}
}

// XoGenAny generates a tuple.
func (Tuple3[A, B, C]) XoGenAny(g *GenRand) any {
	return Tuple3[A, B, C]{Gen[A](g), Gen[B](g), Gen[C](g)}
}

// XoGenAny generates a tuple.
func (Tuple4[A, B, C, D]) XoGenAny(g *GenRand) any {
	return Tuple4[A, B, C, D]{Gen[A](g), Gen[B](g), Gen[C](g), Gen[D](g)}
}

// XoGenAny generates a secret.
func (Secret[T]) XoGenAny(g *GenRand) any { return Secret[T]{v: Gen[T](g)} }

// XoGenAny generates a boxed field value.
func (Box[T]) XoGenAny(g *GenRand) any { return BoxOf(Gen[T](g)) }

// Testing_gen is `testing.gen[T](seed)`.
func Testing_gen[T any](_ *Task, seed int64) T {
	return Gen[T](NewGenRand(uint64(seed)))
}

// PropCase prepares one case of a property test: it generates the inputs
// and returns their Debug text and the test body applied to them.
type PropCase func(g *GenRand) (string, func(*Task, Testing_TestOs) Result[Unit, IError])

// PropInput formats generated inputs as `name = value` lines.
func PropInput(pairs ...any) string {
	var s string
	for i := 0; i+1 < len(pairs); i += 2 {
		s += fmt.Sprintf("%s = %s\n", pairs[i], Debug(pairs[i+1]))
	}
	return s
}

// TestOsAsOs gives a test's fakes as an Os (for contract fuzzing of a
// function that takes `os: Os`).
func TestOsAsOs(t Testing_TestOs) Os {
	return Os{F_net: t.F_net, F_fs: t.F_fs, F_clock: t.F_clock, F_rand: t.F_rand, F_env: t.F_env, F_proc: t.F_proc, F_stdio: t.F_stdio, F_tasks: t.F_tasks}
}
