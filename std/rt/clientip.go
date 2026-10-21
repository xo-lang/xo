package rt

import (
	"net/netip"
	"strings"
)

// Request.client_ip (std.md 4.3): the same steps as std/native/http.xo
// request_client_ip.

// hopAddr parses an address as a peer or a hop gives it: bare, [v6],
// [v6%zone]:port, or v4:port; unmapped.
func hopAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 {
			return netip.Addr{}, false
		}
		s = s[1:end]
	} else if strings.Count(s, ":") == 1 {
		s = s[:strings.Index(s, ":")]
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

func trustedAddr(a netip.Addr, trusted []Netip_Prefix) bool {
	for _, p := range trusted {
		if p.M_contains(nil, netipOf(a)) {
			return true
		}
	}
	return false
}

// splitQuoted splits s on sep outside quoted strings.
func splitQuoted(s string, sep rune) []string {
	var out []string
	var cur strings.Builder
	quoted, esc := false, false
	for _, c := range s {
		switch {
		case esc:
			esc = false
		case quoted && c == '\\':
			esc = true
		case c == '"':
			quoted = !quoted
		case c == sep && !quoted:
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(c)
	}
	return append(out, cur.String())
}

// unquoteParam is a quoted-string's content, or s itself.
func unquoteParam(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	var b strings.Builder
	esc := false
	for _, c := range s[1 : len(s)-1] {
		if !esc && c == '\\' {
			esc = true
			continue
		}
		esc = false
		b.WriteRune(c)
	}
	return b.String()
}

// forwardedFors is the for= values of Forwarded fields, in order (RFC
// 7239): "" for an element without one.
func forwardedFors(fields []string) []string {
	var out []string
	for _, f := range fields {
		for _, e := range splitQuoted(f, ',') {
			v := ""
			for _, p := range splitQuoted(e, ';') {
				if k, val, ok := strings.Cut(p, "="); ok && strings.ToLower(strings.TrimSpace(k)) == "for" {
					v = unquoteParam(val)
				}
			}
			out = append(out, v)
		}
	}
	return out
}

func (r *Http_Request) M_client_ip(tk *Task, trusted List[Netip_Prefix], header Http_ProxyHeader) Option[Netip_Addr] {
	peer, ok := hopAddr(r.F_remote_ip)
	if !ok {
		return Option[Netip_Addr]{}
	}
	if len(trusted.s) == 0 || !trustedAddr(peer, trusted.s) {
		return Some(netipOf(peer))
	}
	var hops []string
	if header.Tag == 1 {
		for _, f := range r.M_header_values(tk, "X-Forwarded-For").s {
			for _, h := range strings.Split(f, ",") {
				hops = append(hops, strings.TrimSpace(h))
			}
		}
	} else {
		hops = forwardedFors(r.M_header_values(tk, "Forwarded").s)
	}
	var leftmost *netip.Addr
	for i := len(hops) - 1; i >= 0; i-- {
		a, ok := hopAddr(hops[i])
		if !ok {
			return Option[Netip_Addr]{}
		}
		if !trustedAddr(a, trusted.s) {
			return Some(netipOf(a))
		}
		leftmost = &a
	}
	if leftmost != nil {
		return Some(netipOf(*leftmost))
	}
	return Some(netipOf(peer))
}
