package rt

// std/smtp on the Go backend (decision 0290): Go's net/smtp SendMail
// (smtp.go of go1.26 with net/textproto's ReadResponse and DotWriter,
// auth.go's PlainAuth and CRAMMD5Auth), ported onto the program's own
// Conn and std/tls rather than net/smtp itself, so FakeNet's connections
// and tls.client's pass through in tests work as natively; the same steps
// and texts as std/native/smtp.xo.

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"net/netip"
	"strconv"
	"strings"
)

func smtpFail(code int64, msg string) Result[Unit, Smtp_SmtpErr] {
	return Err[Unit](Smtp_SmtpErr{F_code: code, F_msg: msg})
}

// smtpErr is a failure of a step.
type smtpErr struct {
	code int64
	msg  string
}

func smtpNetErr(tk *Task, e NetErr) *smtpErr { return &smtpErr{msg: e.M_display(tk)} }

// smtpClient is the connection (plain, then TLS after STARTTLS), the
// bytes read and not yet taken, the EHLO extensions, whether it is TLS,
// and the server's host.
type smtpClient struct {
	tk    *Task
	plain Conn
	t     *Tls_TlsConn
	buf   []byte
	ext   map[string]string
	isTLS bool
	host  string
}

func (c *smtpClient) read() (Option[Bytes], *smtpErr) {
	var r Result[Option[Bytes], NetErr]
	if c.t != nil {
		r = c.t.M_read(c.tk, 4096)
	} else {
		r = c.plain.M_read(c.tk, 4096)
	}
	if !r.Ok {
		return Option[Bytes]{}, smtpNetErr(c.tk, r.E)
	}
	return r.V, nil
}

func (c *smtpClient) write(b []byte) *smtpErr {
	var r Result[Unit, NetErr]
	if c.t != nil {
		r = c.t.M_write(c.tk, Bytes(b))
	} else {
		r = c.plain.M_write(c.tk, Bytes(b))
	}
	if !r.Ok {
		return smtpNetErr(c.tk, r.E)
	}
	return nil
}

func (c *smtpClient) close() {
	if c.t != nil {
		c.t.M_close(c.tk)
	} else {
		c.plain.M_close(c.tk)
	}
}

// line is one line of the server without its CRLF.
func (c *smtpClient) line() (string, *smtpErr) {
	for {
		if i := strings.IndexByte(string(c.buf), '\n'); i >= 0 {
			l := string(c.buf[:i])
			c.buf = c.buf[i+1:]
			return strings.TrimSuffix(l, "\r"), nil
		}
		b, err := c.read()
		if err != nil {
			return "", err
		}
		if !b.Some {
			return "", &smtpErr{msg: "EOF"}
		}
		c.buf = append(c.buf, b.V...)
	}
}

// reply reads a reply (textproto.Reader.ReadResponse): an error unless
// its code starts with the digits of expect (0: any).
func (c *smtpClient) reply(expect int) (int, string, *smtpErr) {
	code, msg, first := 0, "", true
	for {
		l, err := c.line()
		if err != nil {
			return 0, "", err
		}
		if len(l) < 4 || (l[3] != ' ' && l[3] != '-') {
			return 0, "", &smtpErr{msg: "short response: " + l}
		}
		n, e := strconv.Atoi(l[:3])
		if e != nil || n < 100 || n > 999 {
			return 0, "", &smtpErr{msg: "invalid response code: " + l}
		}
		if !first && n != code {
			return 0, "", &smtpErr{msg: "status code mismatch: " + l}
		}
		code = n
		if first {
			msg = l[4:]
		} else {
			msg += "\n" + l[4:]
		}
		first = false
		if l[3] == ' ' {
			break
		}
	}
	if expect > 0 && !strings.HasPrefix(strconv.Itoa(code), strconv.Itoa(expect)) {
		return code, msg, &smtpErr{code: int64(code), msg: msg}
	}
	return code, msg, nil
}

func (c *smtpClient) cmd(expect int, line string) (int, string, *smtpErr) {
	if err := c.write([]byte(line + "\r\n")); err != nil {
		return 0, "", err
	}
	return c.reply(expect)
}

func (c *smtpClient) ehlo(name string) *smtpErr {
	_, msg, err := c.cmd(250, "EHLO "+name)
	if err != nil {
		return err
	}
	ext := map[string]string{}
	lines := strings.Split(msg, "\n")
	for _, l := range lines[1:] {
		if k, v, ok := strings.Cut(l, " "); ok {
			ext[strings.ToUpper(k)] = v
		} else {
			ext[strings.ToUpper(l)] = ""
		}
	}
	c.ext = ext
	return nil
}

func (c *smtpClient) hello(name string) *smtpErr {
	if err := c.ehlo(name); err != nil {
		c.ext = map[string]string{}
		_, _, err := c.cmd(250, "HELO "+name)
		return err
	}
	return nil
}

func smtpValidateLine(s string) *smtpErr {
	if strings.ContainsAny(s, "\r\n") {
		return &smtpErr{msg: "smtp: A line must not contain CR or LF"}
	}
	return nil
}

func smtpLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	return err == nil && a.IsLoopback()
}

func smtpHostOf(addr string) string {
	h := addr
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		h = addr[:i]
	}
	return strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
}

func (c *smtpClient) auth(a Smtp_Auth) *smtpErr {
	if _, ok := c.ext["AUTH"]; !ok {
		return &smtpErr{msg: "smtp: server doesn't support AUTH"}
	}
	switch a.Tag {
	case 2:
		p := a.P.(Smtp_Auth_Plain)
		if !c.isTLS && !smtpLoopback(c.host) {
			return &smtpErr{msg: "unencrypted connection"}
		}
		resp := "\x00" + p.F_user + "\x00" + p.F_password.v
		_, _, err := c.cmd(235, "AUTH PLAIN "+base64.StdEncoding.EncodeToString([]byte(resp)))
		return err
	case 3:
		p := a.P.(Smtp_Auth_CramMd5)
		code, msg, err := c.cmd(0, "AUTH CRAM-MD5")
		if err != nil {
			return err
		}
		if code != 334 {
			return &smtpErr{code: int64(code), msg: msg}
		}
		challenge, e := base64.StdEncoding.DecodeString(msg)
		if e != nil {
			c.cmd(0, "*")
			return &smtpErr{msg: e.Error()}
		}
		d := hmac.New(md5.New, []byte(p.F_secret.v))
		d.Write(challenge)
		answer := p.F_user + " " + hex.EncodeToString(d.Sum(nil))
		_, _, err = c.cmd(235, base64.StdEncoding.EncodeToString([]byte(answer)))
		return err
	}
	return nil
}

// smtpDotStuff is the message as DATA sends it (textproto.DotWriter).
func smtpDotStuff(msg []byte) []byte {
	b := make([]byte, 0, len(msg)+16)
	atStart, prevCR := true, false
	for _, x := range msg {
		if atStart && x == '.' {
			b = append(b, '.')
		}
		if x == '\n' && !prevCR {
			b = append(b, '\r')
		}
		b = append(b, x)
		prevCR = x == '\r'
		atStart = x == '\n'
	}
	if !atStart {
		b = append(b, '\r', '\n')
	}
	return append(b, '.', '\r', '\n')
}

// Smtp_send is `smtp.send(net, addr, from, to, msg, auth, security,
// roots, hello)` (std.md 29).
func Smtp_send(tk *Task, n Net, addr string, from string, to List[string], msg Bytes, auth Smtp_Auth, security Smtp_Security, roots Bytes, hello string) Result[Unit, Smtp_SmtpErr] {
	for _, s := range append(append([]string{from}, to.s...), hello) {
		if err := smtpValidateLine(s); err != nil {
			return smtpFail(err.code, err.msg)
		}
	}
	host := smtpHostOf(addr)
	if security.Tag == 3 && !smtpLoopback(host) {
		return smtpFail(0, "smtp: plain text only to a loopback host (Security.None); use StartTls or Tls")
	}
	var rs Option[Tls_Roots]
	if len(roots) > 0 {
		r := Tls_roots(tk, roots)
		if !r.Ok {
			return smtpFail(0, r.E.F_msg)
		}
		rs = Some(r.V)
	}
	d := n.M_dial(tk, addr)
	if !d.Ok {
		return smtpFail(0, d.E.M_display(tk))
	}
	c := &smtpClient{tk: tk, plain: d.V, host: host, ext: map[string]string{}}
	if security.Tag == 2 {
		t := Tls_client(tk, d.V, host, rs, Tls_Verify{Tag: 1}, List[string]{})
		if !t.Ok {
			d.V.M_close(tk)
			return smtpFail(0, t.E.M_display(tk))
		}
		c.t, c.isTLS = &t.V, true
	}
	err := c.run(from, to.s, msg, auth, security, rs, hello)
	c.close()
	if err != nil {
		return smtpFail(err.code, err.msg)
	}
	return Ok[Unit, Smtp_SmtpErr](Unit{})
}

func (c *smtpClient) run(from string, to []string, msg Bytes, auth Smtp_Auth, security Smtp_Security, rs Option[Tls_Roots], hello string) *smtpErr {
	if _, _, err := c.reply(220); err != nil {
		return err
	}
	if err := c.hello(hello); err != nil {
		return err
	}
	if security.Tag == 1 {
		if _, ok := c.ext["STARTTLS"]; !ok {
			return &smtpErr{msg: "smtp: server doesn't support STARTTLS"}
		}
		if _, _, err := c.cmd(220, "STARTTLS"); err != nil {
			return err
		}
		t := Tls_client(c.tk, c.plain, c.host, rs, Tls_Verify{Tag: 1}, List[string]{})
		if !t.Ok {
			return smtpNetErr(c.tk, t.E)
		}
		c.t, c.isTLS, c.buf = &t.V, true, nil
		if err := c.ehlo(hello); err != nil {
			return err
		}
	}
	if auth.Tag != 1 {
		if err := c.auth(auth); err != nil {
			return err
		}
	}
	mail := "MAIL FROM:<" + from + ">"
	if _, ok := c.ext["8BITMIME"]; ok {
		mail += " BODY=8BITMIME"
	}
	if _, ok := c.ext["SMTPUTF8"]; ok {
		mail += " SMTPUTF8"
	}
	if _, _, err := c.cmd(250, mail); err != nil {
		return err
	}
	for _, r := range to {
		if _, _, err := c.cmd(25, "RCPT TO:<"+r+">"); err != nil {
			return err
		}
	}
	if _, _, err := c.cmd(354, "DATA"); err != nil {
		return err
	}
	if err := c.write(smtpDotStuff([]byte(msg))); err != nil {
		return err
	}
	if _, _, err := c.reply(250); err != nil {
		return err
	}
	_, _, err := c.cmd(221, "QUIT")
	return err
}
