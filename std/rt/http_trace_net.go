//go:build !xo_nohttp

package rt

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
)

// with is ctx carrying r's hooks (ctx itself for a nil recorder).
// WroteHeaderField, Wait100Continue and Got1xxResponse are left out (the
// decision's first set).
func (r *traceRec) with(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			r.add(1, Http_TraceEvent_GetConn{F_host_port: hostPort, F_at: traceNow()})
		},
		GotConn: func(i httptrace.GotConnInfo) {
			r.add(2, Http_TraceEvent_GotConn{F_reused: i.Reused, F_was_idle: i.WasIdle, F_idle: Duration(i.IdleTime), F_at: traceNow()})
		},
		DNSStart: func(i httptrace.DNSStartInfo) {
			r.add(3, Http_TraceEvent_DnsStart{F_host: i.Host, F_at: traceNow()})
		},
		DNSDone: func(i httptrace.DNSDoneInfo) {
			var addrs []string
			for _, a := range i.Addrs {
				addrs = append(addrs, a.String())
			}
			r.add(4, Http_TraceEvent_DnsDone{F_addrs: ListFromSlice(addrs), F_err: traceErr(i.Err), F_at: traceNow()})
		},
		ConnectStart: func(network, addr string) {
			r.add(5, Http_TraceEvent_ConnectStart{F_network: network, F_addr: addr, F_at: traceNow()})
		},
		ConnectDone: func(network, addr string, err error) {
			r.add(6, Http_TraceEvent_ConnectDone{F_network: network, F_addr: addr, F_err: traceErr(err), F_at: traceNow()})
		},
		TLSHandshakeStart: func() {
			r.add(7, Http_TraceEvent_TlsHandshakeStart{F_at: traceNow()})
		},
		TLSHandshakeDone: func(s tls.ConnectionState, err error) {
			v := ""
			if err == nil {
				v = tls.VersionName(s.Version)
			}
			r.add(8, Http_TraceEvent_TlsHandshakeDone{F_version: v, F_alpn: s.NegotiatedProtocol, F_err: traceErr(err), F_at: traceNow()})
		},
		WroteHeaders: func() {
			r.add(9, Http_TraceEvent_WroteHeaders{F_at: traceNow()})
		},
		WroteRequest: func(i httptrace.WroteRequestInfo) {
			r.add(10, Http_TraceEvent_WroteRequest{F_err: traceErr(i.Err), F_at: traceNow()})
		},
		GotFirstResponseByte: func() {
			r.add(11, Http_TraceEvent_GotFirstResponseByte{F_at: traceNow()})
		},
		Got100Continue: func() {
			r.add(12, Http_TraceEvent_Got100Continue{F_at: traceNow()})
		},
		PutIdleConn: func(err error) {
			r.add(13, Http_TraceEvent_PutIdleConn{F_err: traceErr(err), F_at: traceNow()})
		},
	})
}
