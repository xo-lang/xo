package rt

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// Stoppable reads of the real standard input (std.md 3.6, decision 0088).
//
// A read of os.Stdin cannot be interrupted, so a blocked read_line or
// read_all hands the read to one reader goroutine and waits for its
// result, its task's cancellation, or the program's stop request,
// whichever comes first. Cancellation and a stop request return
// IoErr.Canceled. What the goroutine reads after its reader gave up is
// kept (left) for the next read, so no input is lost.
//
// Readers take turns (turn): only the holder touches the state below,
// and the goroutine touches the bufio.Reader only while a request is
// pending. A line already in the buffer is read directly, without the
// goroutine.

type stdinKind int

const (
	stdinLine stdinKind = iota
	stdinAll
)

type stdinResult struct {
	data string
	err  error
}

var stdinState = struct {
	once    sync.Once
	turn    chan struct{}
	req     chan stdinKind
	res     chan stdinResult
	pending bool   // the goroutine is reading
	left    string // read but not delivered
	eof     bool   // the end of input is after left
	err     error  // a read error after left
}{}

func stdinInit() {
	st := &stdinState
	st.once.Do(func() {
		st.turn = make(chan struct{}, 1)
		st.turn <- struct{}{}
		st.req = make(chan stdinKind, 1)
		st.res = make(chan stdinResult, 1)
		go func() {
			for k := range st.req {
				var r stdinResult
				if k == stdinLine {
					r.data, r.err = stdin().ReadString('\n')
				} else {
					var data []byte
					data, r.err = io.ReadAll(stdin())
					r.data = string(data)
					if r.err == nil {
						r.err = io.EOF
					}
				}
				st.res <- r
			}
		}()
	})
}

// stdinStopped reports whether tk may not read: canceled or stop
// requested (core 7.2, 7.5).
func stdinStopped(tk *Task) bool {
	return tk.Canceled() || (tk.stop != nil && tk.stop.requested.Load())
}

// stdinWait waits for c, tk's cancellation, or the stop request; it
// reports whether c was ready.
func stdinWait[T any](tk *Task, c chan T) (T, bool) {
	var zero T
	if stdinStopped(tk) {
		return zero, false
	}
	select {
	case v := <-c:
		return v, true
	case <-tk.Done():
	case <-tk.stopDone():
	}
	return zero, false
}

// stdinRead reads a line (all false) or the rest of the input (all
// true). For a line, ok is false at the end of input.
func stdinRead(tk *Task, all bool) (s string, ok bool, r Result[Unit, IoErr]) {
	canceled := Err[Unit](IoErr{Tag: 3})
	// one leave and enter for the whole read, as for any blocking call:
	// record and replay count them
	// (gotchas/stdin-read-not-interruptible.md)
	tk.leave()
	defer tk.enter()
	stdinInit()
	st := &stdinState
	if _, got := stdinWait(tk, st.turn); !got {
		return "", false, canceled
	}
	defer func() { st.turn <- struct{}{} }()
	for {
		if !all {
			if i := strings.IndexByte(st.left, '\n'); i >= 0 {
				s, st.left = st.left[:i+1], st.left[i+1:]
				return s, true, Ok[Unit, IoErr](Unit{})
			}
		}
		if st.eof || st.err != nil {
			s, st.left = st.left, ""
			if s == "" && st.err != nil {
				err := st.err
				st.err = nil
				return "", false, Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
			}
			return s, all || s != "", Ok[Unit, IoErr](Unit{})
		}
		if !st.pending {
			if !all && st.left == "" {
				// a whole line in the buffer: no need to wait
				r := stdin()
				if b, _ := r.Peek(r.Buffered()); bytes.IndexByte(b, '\n') >= 0 {
					line, _ := r.ReadString('\n')
					return line, true, Ok[Unit, IoErr](Unit{})
				}
			}
			k := stdinLine
			if all {
				k = stdinAll
			}
			st.pending = true
			st.req <- k
		}
		res, got := stdinWait(tk, st.res)
		if !got {
			return "", false, canceled
		}
		st.pending = false
		st.left += res.data
		switch {
		case res.err == io.EOF:
			st.eof = true
		case res.err != nil:
			st.err = res.err
		}
	}
}
