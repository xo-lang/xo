// Command genidna writes the Unicode tables of std/idna into
// std/native/idna.xo (between its BEGIN GENERATED and END GENERATED lines)
// from golang.org/x/net's idna package (the tables for Unicode 15.0.0, which
// go1.26 builds), and prints nothing else. It copies idna's tables, trie and
// trieval files next to dump.go.txt in a temporary module so the dump can read
// their unexported trie, then runs it with the module cache only.
// Usage: go run ./scripts/genidna NETDIR TEXTVERSION FILE
// (NETDIR: the golang.org/x/net@VERSION directory in the module cache;
// TEXTVERSION: the golang.org/x/text version the dump builds with).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: genidna NETDIR TEXTVERSION FILE")
		os.Exit(2)
	}
	netdir, textv, file := os.Args[1], os.Args[2], os.Args[3]
	file, _ = filepath.Abs(file)
	tmp, err := os.MkdirTemp("", "genidna")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	cp := func(from, to string) {
		b, err := os.ReadFile(from)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(to, b, 0o644); err != nil {
			panic(err)
		}
	}
	for _, f := range []string{"tables15.0.0.go", "trie.go", "trieval.go"} {
		cp(filepath.Join(netdir, "idna", f), filepath.Join(tmp, f))
	}
	here, _ := filepath.Abs(filepath.Join("scripts", "genidna", "dump.go.txt"))
	cp(here, filepath.Join(tmp, "dump.go"))
	os.WriteFile(filepath.Join(tmp, "main_test.go"), []byte("package idna\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestDump(t *testing.T) { Dump(os.Getenv(\"GENIDNA_FILE\")) }\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module idnadump\n\ngo 1.26\n\nrequire golang.org/x/text "+textv+"\n"), 0o644)
	cmd := exec.Command("go", "test", "-run", "TestDump", "-count=1", "-mod=mod", ".")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off", "GENIDNA_FILE="+file)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
