package rt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Go calls in record and replay (decision 0093 stage 1, core 11.3).
//
// The adapters written by `xo bind` (internal/gobind) wrap each Go call in
// one of three ways, chosen when the binding is generated:
//
//   - By value (GoFxValue): every parameter and result is a boundary value
//     (scalars, Str, Bytes, Time, Duration, lists and maps of those,
//     go.Error) and the call takes no callback. A recording logs the
//     effect `go.call(<name>, <argument hash>)` with the results; a replay
//     checks the name and hash and returns the recorded results without
//     calling Go. A Go panic is logged as a fault (X "fault") and raised
//     again.
//   - Re executed (GoFxRerun): a result, a parameter, or the receiver is a
//     Go handle, or the call takes an Xo callback. The handle's Go state is
//     live in a replay only if the calls that touch it run, so these calls
//     run again; the trace logs `go.call(<name>)` with X "handle" or
//     "callback", and the replay checks it and reports "re executed
//     foreign call `<name>`" once per name.
//   - Pure (`replay rerun`, no wrapper): pure functions of the std packages
//     gobind lists and the functions an xo.mod `replay rerun go` line
//     names run again in a replay and are not logged.
//
// A trace without the header field "foreign" (written before decision
// 0093) logs no Go calls: its replay runs every Go call again, as before.

// GoFxOn reports whether Go calls may be recorded or replayed: adapters
// take the plain path when it is false.
func GoFxOn() bool { return fxMode != fxOff }

// goFxReplaying reports whether this replay answers Go calls from the
// trace.
func goFxReplaying() bool { return fxMode == fxReplay && rp.foreign }

// goArgHash is the hash of a Go call's arguments that the trace keeps
// (the arguments themselves may be large or sensitive).
func goArgHash(args []any) string {
	enc := make([]any, len(args))
	for i, a := range args {
		enc[i] = fxEncode(a)
	}
	sum := sha256.Sum256([]byte(fxText(enc)))
	return hex.EncodeToString(sum[:8])
}

// GoFxValue runs a Go call answered by value in a replay: name is the Xo
// name (`os.getpid`), args the call's Xo arguments, real the call.
func GoFxValue[R any](tk *Task, name string, args []any, real func() R) R {
	switch {
	case fxMode == fxOff, fxMode == fxReplay && !rp.foreign:
		return real()
	case fxMode == fxReplay:
		h := goArgHash(args)
		// the recording entered and left a Go call here, which is a
		// scheduling point once Go callbacks exist (gointerop.go)
		if tk != nil && tk.th != nil {
			goEnter(tk.th)
		}
		e := rp.next(tk, "go.call", []any{name, h})
		if tk != nil && tk.th != nil {
			goExit(tk.th)
		}
		switch e.X {
		case "fault":
			msg, _ := e.R.(string)
			panic(newFault("", "", msg))
		case "handle", "callback":
			// the binding changed since the recording: run the call
			return real()
		}
		return fxResult[R](e)
	}
	h := goArgHash(args)
	r := goFxRecordReal(tk, name, h, real)
	rec.addX(tk, "go.call", []any{name, h}, r, "")
	return r
}

// goFxRecordReal runs a recorded Go call; a Go panic that became a fault
// is logged before it goes on.
func goFxRecordReal[R any](tk *Task, name, h string, real func() R) R {
	ok := false
	defer func() {
		if ok {
			return
		}
		r := recover()
		if r == nil {
			return
		}
		if f, isFault := r.(*FaultErr); isFault {
			rec.addX(tk, "go.call", []any{name, h}, f.Msg, "fault")
		}
		panic(r)
	}()
	r := real()
	ok = true
	return r
}

// GoFxRerun marks a Go call that runs again in a replay (its result,
// arguments, or receiver hold a Go handle, or it takes a callback): why
// is "handle" or "callback". The adapter calls it before the Go call.
func GoFxRerun(tk *Task, name, why string) {
	switch {
	case fxMode == fxOff, fxMode == fxReplay && !rp.foreign:
		return
	case fxMode == fxReplay:
		rp.next(tk, "go.call", []any{name})
		rp.reexecuted(name, why)
		return
	}
	rec.addX(tk, "go.call", []any{name}, nil, why)
}

// reexecuted reports a Go call that a replay ran again, once per name.
func (p *replayer) reexecuted(name, why string) {
	if _, dup := p.rerun.LoadOrStore(name, true); dup {
		return
	}
	reason := "its result, an argument, or its receiver is a Go handle"
	if why == "callback" {
		reason = "it takes an Xo callback"
	}
	stderrWrite(fmt.Sprintf("xo replay: re executed foreign call `%s` (%s; decision 0093)\n", name, reason))
}

// replayedGoError is the Go error of a go.Error decoded from a trace: its
// text and Go type are the recorded ones.
type replayedGoError string

func (e replayedGoError) Error() string { return string(e) }
