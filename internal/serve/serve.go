// Package serve is the resident checker behind `xo serve`: a process that
// keeps checked programs in memory and answers `xo check` and `xo query`
// over a unix socket, so repeated questions from an agent do not reparse and
// recheck unchanged modules.
//
// The cache holds whole check results, keyed by the working directory and
// the directories checked, and validated by the modification time and size
// of every .xo and xo.mod file under those directories plus every file of
// the user modules the result loaded (imports outside the directories).
// Any change rechecks the whole program; there is no per module
// incremental recheck yet.
package serve

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/query"
)

// SocketEnv names the environment variable that overrides the socket path.
const SocketEnv = "XO_SERVE_SOCKET"

// DisableEnv names the environment variable that, when non empty, makes the
// CLI skip the server and check in process.
const DisableEnv = "XO_NO_SERVE"

// SocketPath is where `xo serve` listens: $XO_SERVE_SOCKET, or
// <user cache dir>/xo/serve.sock.
func SocketPath() (string, error) {
	if p := os.Getenv(SocketEnv); p != "" {
		return p, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xo", "serve.sock"), nil
}

// Request is one message from a client. Op is "run" (Args is an xo command
// line run in Cwd), "ping", "stats", or "shutdown".
type Request struct {
	Op   string   `json:"op"`
	Args []string `json:"args,omitempty"`
	Cwd  string   `json:"cwd,omitempty"`
}

// Response answers a Request.
type Response struct {
	Code   int    `json:"code"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	// Cached is true when every check this request needed was served from
	// the cache.
	Cached bool   `json:"cached"`
	Error  string `json:"error,omitempty"`
	Stats  *Stats `json:"stats,omitempty"`
}

// Stats counts cache use since the server started.
type Stats struct {
	Hits    int `json:"hits"`
	Misses  int `json:"misses"`
	Entries int `json:"entries"`
}

// ---------------------------------------------------------------------------
// cache

type fileStamp struct {
	mod  time.Time
	size int64
}

type entry struct {
	sess  *query.Session
	files map[string]fileStamp // absolute path -> stamp
	dirs  []string             // absolute directories walked
}

// maxEntries bounds the cache; the least recently used entry is dropped.
const maxEntries = 16

// Cache holds checked sessions keyed by working directory and directories.
type Cache struct {
	mu      sync.Mutex
	entries map[string]*entry
	used    map[string]time.Time
	stats   Stats
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{entries: map[string]*entry{}, used: map[string]time.Time{}}
}

// Load returns the checked session for dirs (relative to the current
// directory), reusing the cached one when no source file changed. hit
// reports whether it was reused.
func (c *Cache) Load(dirs []string) (sess *query.Session, hit bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, false, err
	}
	key := cwd + "\x00" + strings.Join(dirs, "\x00")
	c.mu.Lock()
	e := c.entries[key]
	c.mu.Unlock()
	if e != nil && e.fresh() {
		c.mu.Lock()
		c.stats.Hits++
		c.used[key] = time.Now()
		c.mu.Unlock()
		return e.sess, true, nil
	}
	sess, err = query.Load(dirs)
	if err != nil {
		return nil, false, err
	}
	e = &entry{sess: sess}
	for _, d := range dirs {
		if ad, err := filepath.Abs(d); err == nil {
			e.dirs = append(e.dirs, ad)
		}
	}
	e.files = e.snapshot(sess)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Misses++
	c.entries[key] = e
	c.used[key] = time.Now()
	if len(c.entries) > maxEntries {
		oldest, when := "", time.Time{}
		for k, t := range c.used {
			if oldest == "" || t.Before(when) {
				oldest, when = k, t
			}
		}
		delete(c.entries, oldest)
		delete(c.used, oldest)
	}
	return sess, false, nil
}

// Stats returns the cache counters.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats
	s.Entries = len(c.entries)
	return s
}

// snapshot stamps the source files under the entry's directories and the
// files of every user module sess loaded.
func (e *entry) snapshot(sess *query.Session) map[string]fileStamp {
	out := e.walk()
	if sess != nil && sess.Info != nil {
		for _, m := range sess.Info.AllModules() {
			if m.IsStd {
				continue
			}
			for _, f := range m.Files {
				ap, err := filepath.Abs(f.Path)
				if err != nil {
					continue
				}
				if st, err := os.Stat(ap); err == nil {
					out[ap] = fileStamp{st.ModTime(), st.Size()}
				}
			}
			if m.Dir != "" {
				// xo.mod, and the module files that change what imports
				// resolve to (decision 0086): xo.lock, vendor/modules.txt
				for _, name := range []string{"xo.mod", "xo.lock", filepath.Join("vendor", "modules.txt")} {
					if ap, err := filepath.Abs(filepath.Join(m.Dir, name)); err == nil {
						if st, err := os.Stat(ap); err == nil {
							out[ap] = fileStamp{st.ModTime(), st.Size()}
						}
					}
				}
				if root := mod.ModRoot(m.Dir); root != "" {
					if w, _ := mod.FindWork(root); w != nil {
						if st, err := os.Stat(w.File); err == nil {
							out[w.File] = fileStamp{st.ModTime(), st.Size()}
						}
					}
				}
			}
		}
	}
	return out
}

// walk stamps every .xo and xo.mod file under the entry's directories,
// skipping dot directories as module loading does.
func (e *entry) walk() map[string]fileStamp {
	out := map[string]fileStamp{}
	for _, root := range e.dirs {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".xo") || d.Name() == "xo.mod" || d.Name() == "xo.lock" || d.Name() == "xo.work" {
				if st, err := d.Info(); err == nil {
					out[path] = fileStamp{st.ModTime(), st.Size()}
				}
			}
			return nil
		})
	}
	return out
}

// fresh reports whether no stamped file changed and no source file
// appeared or disappeared under the directories.
func (e *entry) fresh() bool {
	for p, s := range e.files {
		st, err := os.Stat(p)
		if err != nil || !st.ModTime().Equal(s.mod) || st.Size() != s.size {
			return false
		}
	}
	for p := range e.walk() {
		if _, ok := e.files[p]; !ok {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// server

// Runner runs one xo command line with the given session loader and
// returns its exit status.
type Runner func(args []string, load func([]string) (*query.Session, error), stdout, stderr io.Writer) int

// Server answers requests on a listener.
type Server struct {
	Cache *Cache
	Run   Runner

	runMu sync.Mutex // one command at a time (commands may change directory)
	done  chan struct{}
	once  sync.Once
}

// NewServer returns a server running commands with run.
func NewServer(run Runner) *Server {
	return &Server{Cache: NewCache(), Run: run, done: make(chan struct{})}
}

// Listen opens the unix socket at path, replacing a stale socket file left
// by a server that is no longer running. It fails when a server answers.
func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		if _, err := Call(path, Request{Op: "ping"}); err == nil {
			return nil, fmt.Errorf("a server is already listening on %s", path)
		}
		os.Remove(path)
	}
	return net.Listen("unix", path)
}

// Serve accepts connections until a shutdown request arrives or ln fails.
func (s *Server) Serve(ln net.Listener) error {
	go func() {
		<-s.done
		ln.Close()
	}()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handle(conn)
		}()
	}
}

func (s *Server) stop() { s.once.Do(func() { close(s.done) }) }

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	var req Request
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err == nil || len(line) > 0 {
		err = json.Unmarshal(line, &req)
	}
	var resp Response
	switch {
	case err != nil:
		resp = Response{Code: 2, Error: "bad request: " + err.Error()}
	case req.Op == "ping":
	case req.Op == "stats":
		st := s.Cache.Stats()
		resp.Stats = &st
	case req.Op == "shutdown":
		defer s.stop()
	case req.Op == "run":
		resp = s.run(req)
	default:
		resp = Response{Code: 2, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}

func (s *Server) run(req Request) (resp Response) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if req.Cwd != "" {
		old, err := os.Getwd()
		if err != nil {
			return Response{Code: 2, Error: err.Error()}
		}
		if old != req.Cwd {
			if err := os.Chdir(req.Cwd); err != nil {
				return Response{Code: 2, Error: err.Error()}
			}
			defer os.Chdir(old)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			resp = Response{Code: 2, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	loads, hits := 0, 0
	load := func(dirs []string) (*query.Session, error) {
		sess, hit, err := s.Cache.Load(dirs)
		loads++
		if hit {
			hits++
		}
		return sess, err
	}
	var out, errOut strings.Builder
	code := s.Run(req.Args, load, &out, &errOut)
	return Response{Code: code, Stdout: out.String(), Stderr: errOut.String(), Cached: loads > 0 && hits == loads}
}

// ---------------------------------------------------------------------------
// client

// dialTimeout bounds the connection attempt so a missing server costs
// little; callTimeout bounds a whole request.
const (
	dialTimeout = 300 * time.Millisecond
	callTimeout = 10 * time.Minute
)

// Call sends req to the server at path and returns its response.
func Call(path string, req Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(callTimeout))
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Forward runs args on the server at the default socket path when one
// answers. ok is false when no server is reachable (or DisableEnv is set),
// and the caller should run the command itself.
func Forward(args []string, stdout, stderr io.Writer) (code int, ok bool) {
	if os.Getenv(DisableEnv) != "" {
		return 0, false
	}
	// module settings of this process (decision 0086) that the server's
	// environment may not share: check in process
	if os.Getenv("XO_OFFLINE") != "" || os.Getenv("XO_WORK") != "" || os.Getenv("XO_MODCACHE") != "" || os.Getenv("XO_GIT_BASE") != "" || os.Getenv("XO_GIT_REWRITE") != "" {
		return 0, false
	}
	path, err := SocketPath()
	if err != nil {
		return 0, false
	}
	if _, err := os.Stat(path); err != nil {
		return 0, false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, false
	}
	resp, err := Call(path, Request{Op: "run", Args: args, Cwd: cwd})
	if err != nil || resp.Error != "" {
		return 0, false
	}
	io.WriteString(stdout, resp.Stdout)
	io.WriteString(stderr, resp.Stderr)
	return resp.Code, true
}
