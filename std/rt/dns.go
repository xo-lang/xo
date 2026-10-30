package rt

import (
	"context"
	"errors"
	"net"
	"strings"
	"unicode/utf8"
)

// std/dns (decision 0417): Go's LookupMX, LookupTXT, LookupSRV and
// LookupAddr over a Net, with Net.only's check of the name and Ok with
// the valid records when Go returns some together with its
// errMalformedDNSRecordsDetail error.

// dnsResolver is the net.Resolver of r: the system's, or one that sends
// every query to r.server (Go's pure resolver dialing it).
func dnsResolver(r *Dns_Resolver) *net.Resolver {
	if r == nil || r.F_server == "" {
		return net.DefaultResolver
	}
	server := r.F_server
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, server)
	}}
}

// dnsCheck is the common start of a lookup: canceled, Denied by
// Net.only for subject, or FakeNet without --real ("no such host").
// It returns the Net to look up through (FakeNet's real one).
func dnsCheck(tk *Task, n Net, subject, name string) *NetErr {
	if tk.Canceled() {
		e := netErr(5, "")
		return &e
	}
	for {
		switch v := n.(type) {
		case *onlyNet:
			if !v.admits(strings.ToLower(strings.TrimSuffix(subject, ".")), "0") {
				e := netDenied(subject)
				return &e
			}
			n = v.inner
			continue
		case Testing_FakeNet:
			if v.n.real == nil {
				e := netErr(4, "lookup "+name+": no such host")
				return &e
			}
		}
		return nil
	}
}

// dnsErr is a lookup's NetErr: Io with Go's DNSError text.
func dnsErr(err error) NetErr {
	var de *net.DNSError
	if errors.As(err, &de) && de.Err == "unrecognized address" {
		return netErr(4, "lookup "+de.Name+": unrecognized address")
	}
	return netErr(4, err.Error())
}

// dnsPartial reports whether err is Go's error for records it left out
// as invalid, returned with the valid ones.
func dnsPartial(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && strings.Contains(de.Err, "invalid names")
}

func (r *Dns_Resolver) M_lookup_mx(tk *Task, n Net, name string) Result[List[Dns_Mx], NetErr] {
	if e := dnsCheck(tk, n, name, name); e != nil {
		return Err[List[Dns_Mx]](*e)
	}
	mxs, err := dnsResolver(r).LookupMX(context.Background(), name)
	if err != nil && (!dnsPartial(err) || len(mxs) == 0) {
		return Err[List[Dns_Mx]](dnsErr(err))
	}
	out := make([]Dns_Mx, len(mxs))
	for i, m := range mxs {
		out[i] = Dns_Mx{F_host: m.Host, F_pref: int64(m.Pref)}
	}
	return Ok[List[Dns_Mx], NetErr](ListFromSlice(out))
}

func (r *Dns_Resolver) M_lookup_txt(tk *Task, n Net, name string) Result[List[string], NetErr] {
	if e := dnsCheck(tk, n, name, name); e != nil {
		return Err[List[string]](*e)
	}
	txts, err := dnsResolver(r).LookupTXT(context.Background(), name)
	if err != nil {
		return Err[List[string]](dnsErr(err))
	}
	out := txts[:0:0]
	for _, t := range txts {
		if utf8.ValidString(t) {
			out = append(out, t)
		}
	}
	if len(out) == 0 && len(txts) > 0 {
		return Err[List[string]](netErr(4, "lookup "+name+": DNS response contained records which are not UTF-8"))
	}
	return Ok[List[string], NetErr](ListFromSlice(out))
}

func (r *Dns_Resolver) M_lookup_srv(tk *Task, n Net, service, proto, name string) Result[List[Dns_Srv], NetErr] {
	target := name
	if service != "" || proto != "" {
		target = "_" + service + "._" + proto + "." + name
	}
	if e := dnsCheck(tk, n, name, target); e != nil {
		return Err[List[Dns_Srv]](*e)
	}
	_, srvs, err := dnsResolver(r).LookupSRV(context.Background(), service, proto, name)
	if err != nil && (!dnsPartial(err) || len(srvs) == 0) {
		return Err[List[Dns_Srv]](dnsErr(err))
	}
	out := make([]Dns_Srv, len(srvs))
	for i, s := range srvs {
		out[i] = Dns_Srv{F_target: s.Target, F_port: int64(s.Port), F_priority: int64(s.Priority), F_weight: int64(s.Weight)}
	}
	return Ok[List[Dns_Srv], NetErr](ListFromSlice(out))
}

func (r *Dns_Resolver) M_lookup_addr(tk *Task, n Net, ip string) Result[List[string], NetErr] {
	if e := dnsCheck(tk, n, ip, ip); e != nil {
		return Err[List[string]](*e)
	}
	names, err := dnsResolver(r).LookupAddr(context.Background(), ip)
	if err != nil && (!dnsPartial(err) || len(names) == 0) {
		return Err[List[string]](dnsErr(err))
	}
	return Ok[List[string], NetErr](ListFromSlice(names))
}

// Dns_lookup_mx is `dns.lookup_mx(n, name)`: the system's servers.
func Dns_lookup_mx(tk *Task, n Net, name string) Result[List[Dns_Mx], NetErr] {
	return (&Dns_Resolver{}).M_lookup_mx(tk, n, name)
}

// Dns_lookup_txt is `dns.lookup_txt(n, name)`.
func Dns_lookup_txt(tk *Task, n Net, name string) Result[List[string], NetErr] {
	return (&Dns_Resolver{}).M_lookup_txt(tk, n, name)
}

// Dns_lookup_srv is `dns.lookup_srv(n, service, proto, name)`.
func Dns_lookup_srv(tk *Task, n Net, service, proto, name string) Result[List[Dns_Srv], NetErr] {
	return (&Dns_Resolver{}).M_lookup_srv(tk, n, service, proto, name)
}

// Dns_lookup_addr is `dns.lookup_addr(n, ip)`.
func Dns_lookup_addr(tk *Task, n Net, ip string) Result[List[string], NetErr] {
	return (&Dns_Resolver{}).M_lookup_addr(tk, n, ip)
}
