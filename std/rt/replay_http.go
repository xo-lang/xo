//go:build !xo_nohttp

package rt

import (
	"fmt"
	"reflect"
	"sync"
)

// newReq logs a request that reached the server of listener l and
// returns its id.
func (r *recorder) newReq(l int64, req Http_Request) int64 {
	k := fxReqSeq.Add(1)
	e := &reqEntry{Req: k, L: l, R: fxEncode(req)}
	r.mu.Lock()
	if r.full {
		r.reqs[k] = e
	} else {
		r.rings[k] = &ring{req: e}
	}
	r.mu.Unlock()
	return k
}

// reqResp logs the response to request k.
func (r *recorder) reqResp(k int64, resp Http_Response) {
	enc := fxEncode(resp)
	r.mu.Lock()
	if e := r.reqs[k]; e != nil {
		e.Resp = enc
	}
	if rg := r.rings[k]; rg != nil && rg.req != nil {
		rg.req.Resp = enc
	}
	r.mu.Unlock()
}

// rpServer is a server of a replay: its requests come from the trace.
type rpServer struct {
	router  Http_Router
	tk      *Task
	reqRoot *cnode
	ended   chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

// fireRequest delivers recorded request k to its server.
func (p *replayer) fireRequest(k int64) {
	p.mu.Lock()
	re := p.reqs[k]
	var srv *rpServer
	if re != nil {
		srv = p.servers[re.L]
	}
	p.mu.Unlock()
	if re == nil {
		p.fail(fmt.Sprintf("the trace delivers request %d but does not record it", k))
	}
	if srv == nil {
		p.diverge(fmt.Sprintf("the trace delivers request %d to listener %d, but the program does not serve it", k, re.L))
	}
	v, err := fxDecode(re.R, reflect.TypeFor[Http_Request]())
	if err != nil {
		p.fail(fmt.Sprintf("request %d cannot be decoded: %v", k, err))
	}
	req := v.Interface().(Http_Request)
	stk := srv.tk
	reqTask := &Task{stop: stk.stop, th: stk.th.newChild("request")}
	th := reqTask.th
	th.req = k
	node := newNodeOf(srv.reqRoot, nil, nodeRequest, reqTask)
	reqTask.node = node
	if th.mon != nil {
		th.mon.started()
	}
	srv.wg.Add(1)
	if sch := th.sch; sch != nil {
		sch.start(th)
	}
	go func() {
		defer srv.wg.Done()
		defer node.detach()
		if sch := th.sch; sch != nil {
			<-th.turn
		}
		defer endThread(th)
		resp := srv.router.call(reqTask, req).collect(reqTask)
		p.checkResp(reqTask, k, re, resp)
	}()
}

// checkResp compares a replayed response with the recorded one.
func (p *replayer) checkResp(tk *Task, k int64, re *reqEntry, resp Http_Response) {
	if re.Resp == nil {
		return
	}
	got := fxText(fxEncode(resp))
	if want := fxText(re.Resp); got != want {
		p.diverge(fmt.Sprintf("the response to request %d differs:\n  replayed %s\n  recorded %s", k, clip(got), clip(want)))
	}
}

// replayServe is srv.serve() under a replay: the requests come from the
// trace, and the serve ends when the trace says it did.
func replayServe(tk *Task, s Http_Server) Result[Unit, Http_HttpErr] {
	p := rp
	srv := &rpServer{router: s.r, tk: tk, reqRoot: newNodeOf(nil, tk.w, nodeServer, nil), ended: make(chan struct{})}
	extraRoots.Store(srv.reqRoot, true)
	defer extraRoots.Delete(srv.reqRoot)
	p.addServer(s.l.id, srv)
	tk.leave()
	if p.full {
		<-srv.ended
	} else {
		// ring buffers: deliver the failing request of the bundle
		var focus *reqEntry
		p.mu.Lock()
		for _, re := range p.reqs {
			if re.L == s.l.id {
				focus = re
			}
		}
		p.mu.Unlock()
		if focus != nil {
			p.fireRequest(focus.Req)
		}
		srv.wg.Wait()
	}
	tk.enter()
	e := p.next(tk, "http.serve", []any{s.l.id})
	return fxResult[Result[Unit, Http_HttpErr]](e)
}
