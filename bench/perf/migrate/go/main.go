// Go twin of bench/perf/migrate: goose's sqlparser over the same files,
// Up then Down, as goose reads a migration.
package main

import (
	"fmt"
	"os"
	"strings"

	"twincheck.local/migratetwin/sqlparser"
)

func file(i int) string {
	notx := ""
	if i%5 == 0 {
		notx = "-- +goose NO TRANSACTION\n"
	}
	return fmt.Sprintf("%s-- +goose Up\nCREATE TABLE t%d (\n  id INTEGER PRIMARY KEY,\n  name TEXT NOT NULL\n);\nINSERT INTO t%d (id, name) VALUES (1, 'a;b');\n-- +goose StatementBegin\nCREATE FUNCTION f%d() RETURNS int AS $$\nBEGIN\n  RETURN 1;\nEND;\n$$ LANGUAGE plpgsql;\n-- +goose StatementEnd\n\n-- +goose Down\nDROP FUNCTION f%d;\nDROP TABLE t%d;\n", notx, i, i, i, i, i)
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	files := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		files = append(files, file(i))
	}
	sum := 0
	if w == "parse" {
		for r := 0; r < 2000; r++ {
			for _, t := range files {
				up, tx, err := sqlparser.ParseSQLMigration(strings.NewReader(t), sqlparser.DirectionUp, false)
				down, _, err2 := sqlparser.ParseSQLMigration(strings.NewReader(t), sqlparser.DirectionDown, false)
				if err != nil || err2 != nil {
					sum += 100000
					continue
				}
				sum += len(up)*7 + len(down)*3
				if tx {
					sum++
				}
				for _, s := range up {
					sum += len(s)
				}
				for _, s := range down {
					sum += len(s)
				}
			}
		}
	} else {
		for _, t := range files {
			sum += len(t)
		}
	}
	fmt.Printf("%s %d\n", w, sum)
}
