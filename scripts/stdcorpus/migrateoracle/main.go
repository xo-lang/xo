// migrateoracle writes the std/migrate fuzz corpus (decision 0616's fuzz
// kind, task #233): seeded random goose migration files (Up and Down
// sections, single and multi-line statements, StatementBegin and End
// blocks with semicolons inside, comments, NO TRANSACTION, ENVSUB ON and
// OFF with ${VAR} references), most changed at the line level (a line
// dropped, doubled or moved, an annotation's case or spacing changed, a
// semicolon dropped or added, the file cut short), and random file names.
// The reference is pressly/goose v3.27.3's own parser (sqlparser, copied:
// an internal package) and NumericComponent (below): a file it parses must
// give std/migrate.parse the same Up and Down statements and transaction
// mode, a file it refuses must be refused, and a name must give
// version_of the same version.
//
// XO_MIGRATEFUZZ_SEED (default 0, the committed corpus) moves the seed and
// XO_MIGRATEFUZZ_N (default 300) sets how many files are drawn;
// internal/stdtest's TestMigrateFuzz runs a fresh seed.
package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"stdcorpus/migrateoracle/sqlparser"
)

func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func xlist(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = xq(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// numericComponent is goose v3.27.3's NumericComponent (migration.go),
// copied to keep goose's database drivers out of this module. Copyright
// (c) 2012 Liam Staskawicz, MIT license.
func numericComponent(filename string) (int64, error) {
	base := filepath.Base(filename)
	if ext := filepath.Ext(base); ext != ".go" && ext != ".sql" {
		return 0, errors.New("migration file does not have .sql or .go file extension")
	}
	before, _, ok := strings.Cut(base, "_")
	if !ok {
		return 0, errors.New("no filename separator '_' found")
	}
	n, err := strconv.ParseInt(before, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse version from migration file: %s: %w", base, err)
	}
	if n < 1 {
		return 0, errors.New("migration version must be greater than zero")
	}
	return n, nil
}

// ENV is the environment both sides read: goose's envsub reads the
// process environment, std/migrate.parse takes it as a map
var ENV = map[string]string{"XO_MF_A": "alpha", "XO_MF_B": "b$x", "XO_MF_EMPTY": ""}

var words = []string{"CREATE TABLE t (id INT)", "INSERT INTO t VALUES (1)", "DROP TABLE t", "SELECT 'a;b'", "UPDATE t SET x = 'it''s'", "ALTER TABLE t ADD COLUMN c TEXT", "SELECT ${XO_MF_A}", "SELECT '${XO_MF_B}'", "SELECT $1", "SELECT ${XO_MF_MISSING:-dflt}", "SELECT ${XO_MF_EMPTY}x", "SELECT \\${XO_MF_A}", "SELECT 1 -- trailing", "CREATE INDEX i ON t (c)"}

func stmt(r *rand.Rand) []string {
	n := 1 + r.Intn(3)
	var lines []string
	for i := 0; i < n; i++ {
		lines = append(lines, words[r.Intn(len(words))])
	}
	lines[len(lines)-1] += ";"
	return lines
}

func block(r *rand.Rand) []string {
	l := []string{"-- +goose StatementBegin", "CREATE FUNCTION f() RETURNS INT AS $$", "BEGIN"}
	for i := 0; i < 1+r.Intn(3); i++ {
		l = append(l, "  "+words[r.Intn(len(words))]+";")
	}
	l = append(l, "END;", "$$ LANGUAGE plpgsql;", "-- +goose StatementEnd")
	return l
}

func section(r *rand.Rand, dir string) []string {
	l := []string{"-- +goose " + dir}
	for i := 0; i < r.Intn(4); i++ {
		switch r.Intn(7) {
		case 0:
			l = append(l, block(r)...)
		case 1:
			l = append(l, "-- a comment", "")
		case 2:
			if r.Intn(2) == 0 {
				l = append(l, "-- +goose ENVSUB ON")
			} else {
				l = append(l, "-- +goose ENVSUB OFF")
			}
		default:
			l = append(l, stmt(r)...)
		}
	}
	return l
}

func file(r *rand.Rand) []string {
	var l []string
	if r.Intn(4) == 0 {
		l = append(l, "-- +goose NO TRANSACTION")
	}
	if r.Intn(5) == 0 {
		l = append(l, "-- leading comment")
	}
	l = append(l, section(r, "Up")...)
	if r.Intn(4) > 0 {
		l = append(l, "")
		l = append(l, section(r, "Down")...)
	}
	return l
}

var annos = []string{"-- +goose Up", "-- +goose Down", "-- +goose StatementBegin", "-- +goose StatementEnd", "-- +goose NO TRANSACTION", "-- +goose ENVSUB ON", "-- +goose ENVSUB OFF", "-- +goose up", "--+goose Up", "-- +goose  Down", "-- +goose Unknown", "-- +goose", "--  +goose StatementEnd", "-- +goose Up -- x"}

func mutate(r *rand.Rand, l []string) []string {
	if len(l) == 0 {
		return l
	}
	i := r.Intn(len(l))
	switch r.Intn(8) {
	case 0:
		return append(l[:i:i], l[i+1:]...)
	case 1:
		return append(l[:i+1:i+1], l[i:]...)
	case 2:
		j := r.Intn(len(l))
		l[i], l[j] = l[j], l[i]
	case 3:
		return append(l[:i:i], append([]string{annos[r.Intn(len(annos))]}, l[i:]...)...)
	case 4:
		l[i] = strings.TrimSuffix(l[i], ";")
	case 5:
		l[i] += ";"
	case 6:
		l[i] = strings.ToLower(l[i])
	default:
		return l[:i]
	}
	return l
}

var names = []string{"001_init.sql", "1_a.sql", "0_zero.sql", "-3_neg.sql", "+4_plus.sql", "20260101120000_ts.sql", "99999999999999999999_big.sql", "x_bad.sql", "noversion.sql", "5.sql", "6_.sql", "_7.sql", "8__two.sql", "9_a.SQL", "10_a.go", "11_a.txt", "dir/12_nested.sql", "13_a b.sql", "0x1f_hex.sql", "1e3_exp.sql", " 14_space.sql", "15 _x.sql", "000016_pad.sql"}

func main() {
	seed := int64(envInt("XO_MIGRATEFUZZ_SEED", 0))
	n := envInt("XO_MIGRATEFUZZ_N", 300)
	for k, v := range ENV {
		os.Setenv(k, v)
	}
	os.Unsetenv("XO_MF_MISSING")
	r := rand.New(rand.NewSource(seed*7919 + 3))
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from pressly/goose v3.27.3's sqlparser and NumericComponent (" + os.Args[1] + ").\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus migratefuzz.\n")
	o.WriteString("use std/migrate\n\n")
	fmt.Fprintf(&o, "/// seed %d: (source, parsed by goose, its Up, its Down, its transaction mode)\nconst FILES: List[(Str, Bool, List[Str], List[Str], Bool)] = [\n", seed)
	acc := 0
	for i := 0; i < n; i++ {
		l := file(r)
		for k := 0; k < r.Intn(3) && i%4 != 0; k++ {
			l = mutate(r, l)
		}
		src := strings.Join(l, "\n")
		if r.Intn(3) > 0 {
			src += "\n"
		}
		up, tx, err := sqlparser.ParseSQLMigration(strings.NewReader(src), sqlparser.DirectionUp, false)
		var down []string
		if err == nil {
			down, _, err = sqlparser.ParseSQLMigration(strings.NewReader(src), sqlparser.DirectionDown, false)
		}
		if err != nil {
			fmt.Fprintf(&o, "  (%s, false, [], [], false),\n", xq(src))
			continue
		}
		acc++
		fmt.Fprintf(&o, "  (%s, true, %s, %s, %v),\n", xq(src), xlist(up), xlist(down), tx)
	}
	o.WriteString("]\n\n/// (file name, goose's version or 0 when it has none)\nconst NAMES: List[(Str, Int)] = [\n")
	for _, nm := range names {
		v, err := numericComponent(nm)
		if err != nil || filepath.Ext(nm) != ".sql" {
			v = 0
		}
		fmt.Fprintf(&o, "  (%s, %d),\n", xq(nm), v)
	}
	o.WriteString("]\n")
	o.WriteString("\nconst ENV: Map[Str, Str] = {")
	for i, k := range []string{"XO_MF_A", "XO_MF_B", "XO_MF_EMPTY"} {
		if i > 0 {
			o.WriteString(", ")
		}
		fmt.Fprintf(&o, "%s: %s", xq(k), xq(ENV[k]))
	}
	o.WriteString("}\n")
	o.WriteString(xoTests)
	fmt.Fprintf(os.Stderr, "migrateoracle: seed %d, %d files, %d parsed by goose\n", seed, n, acc)
	os.Stdout.WriteString(o.String())
}

const xoTests = `
test "std/migrate.parse agrees with goose's parser" {
  var bad: List[Str] = []
  for (src, ok, up, down, tx) in FILES {
    match migrate.parse("1_f.sql", src, ENV) {
      Ok(m) => {
        if !ok {
          bad.push("goose refuses, parse accepts:\n${src}")
        } else if m.up != up || m.down != down || m.tx != tx {
          bad.push("differs:\n${src}\ngot up ${m.up} down ${m.down} tx ${m.tx}\nwant up ${up} down ${down} tx ${tx}")
        }
      }
      Err(e) => {
        if ok { bad.push("goose parses, parse refuses (${e.display()}):\n${src}") }
      }
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${FILES.len()} files differ, first: ${bad[0]}")
  }
}

test "std/migrate.version_of agrees with goose's NumericComponent" {
  for (name, want) in NAMES {
    let got = migrate.version_of(name) ?? 0
    if got != want {
      fault("version_of(${name}) = ${got}, goose ${want}")
    }
  }
}
`
