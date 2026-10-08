package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/xo-lang/xo/internal/mcp"
	"github.com/xo-lang/xo/internal/query"
	"github.com/xo-lang/xo/internal/serve"
)

// loader checks the modules under dirs: query.Load in process, or the
// cache of `xo serve`.
type loader func(dirs []string) (*query.Session, error)

// ---------------------------------------------------------------------------
// mcp

func runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fl.SetOutput(stderr)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	s := &mcp.Server{Exec: run, Version: "p1"}
	if err := s.Serve(stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "xo mcp: %v\n", err)
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// serve

// serveRun runs the commands `xo serve` answers, with the cache's loader.
func serveRun(args []string, load func([]string) (*query.Session, error), stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "xo serve: empty command")
		return 2
	}
	switch args[0] {
	case "check":
		return runCheckWith(args[1:], load, stdout, stderr)
	case "query":
		if len(args) == 1 {
			fmt.Fprint(stderr, queryUsage)
			return 2
		}
		return runQueryWith(args[1:], load, stdout, stderr)
	}
	fmt.Fprintf(stderr, "xo serve: %s is not served\n", args[0])
	return 2
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	fl.SetOutput(stderr)
	sock := fl.String("socket", "", "unix socket path (default $XO_SERVE_SOCKET or <user cache dir>/xo/serve.sock)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	path := *sock
	if path == "" {
		p, err := serve.SocketPath()
		if err != nil {
			fmt.Fprintf(stderr, "xo serve: %v\n", err)
			return 2
		}
		path = p
	}
	switch fl.Arg(0) {
	case "stop", "status":
		op := "shutdown"
		if fl.Arg(0) == "status" {
			op = "stats"
		}
		resp, err := serve.Call(path, serve.Request{Op: op})
		if err != nil {
			fmt.Fprintf(stderr, "xo serve: no server on %s\n", path)
			return 1
		}
		if resp.Stats != nil {
			if err := writeJSON(stdout, resp.Stats); err != nil {
				return 2
			}
		}
		return 0
	case "":
	default:
		fmt.Fprintf(stderr, "xo serve: unknown subcommand %q (want stop or status)\n", fl.Arg(0))
		return 2
	}
	ln, err := serve.Listen(path)
	if err != nil {
		fmt.Fprintf(stderr, "xo serve: %v\n", err)
		return 1
	}
	defer os.Remove(path)
	fmt.Fprintf(stderr, "xo serve: listening on %s (pid %d); `xo serve stop` stops it\n", path, os.Getpid())
	if err := serve.NewServer(serveRun).Serve(ln); err != nil {
		fmt.Fprintf(stderr, "xo serve: %v\n", err)
		return 1
	}
	return 0
}
