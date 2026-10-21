package main

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// smtpScript is a fake server's replies; each reply's lines are joined
// with "\n" (written with CRLF).
type smtpScript struct {
	greet, ext, helo, auth, auth2, mail, rcpts, data, end, quit string
	ehloOK                                                      bool
}

// smtpServe answers one session on c as the Xo test's serve does: the
// client's lines (the DATA payload as one line of |-joined lines after
// "== data").
func smtpServe(c net.Conn, s smtpScript) []string {
	defer c.Close()
	// a script that leaves the client waiting ends here instead of hanging
	c.SetDeadline(time.Now().Add(3 * time.Second))
	var seen []string
	write := func(r string) {
		for _, l := range strings.Split(r, "\n") {
			c.Write([]byte(l + "\r\n"))
		}
	}
	write(s.greet)
	rd := bufio.NewReader(c)
	rcpts := strings.Split(s.rcpts, ";")
	nrcpt := 0
	inData := false
	data := ""
	authed := false
	for {
		line, err := rd.ReadString('\n')
		if errors.Is(err, os.ErrDeadlineExceeded) {
			panic(fmt.Sprintf("smtp corpus: a script leaves the client waiting (change the generator): %+v", s))
		}
		if err != nil {
			return seen
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if inData {
			if line == "." {
				inData = false
				seen = append(seen, "== data", data)
				write(s.end)
			} else {
				data += line + "|"
			}
			continue
		}
		seen = append(seen, line)
		var reply string
		switch {
		case strings.HasPrefix(line, "EHLO"):
			if !s.ehloOK {
				reply = "502 5.5.1 no EHLO"
			} else {
				lines := []string{"mail.test"}
				if s.ext != "" {
					lines = append(lines, strings.Split(s.ext, ";")...)
				}
				var rs []string
				for k, e := range lines {
					sep := "-"
					if k == len(lines)-1 {
						sep = " "
					}
					rs = append(rs, "250"+sep+e)
				}
				reply = strings.Join(rs, "\n")
			}
		case strings.HasPrefix(line, "HELO"):
			reply = s.helo
		case strings.HasPrefix(line, "AUTH"):
			reply = s.auth
			// a 334 challenge: the next line answers it
			authed = strings.HasPrefix(s.auth, "334")
		case authed && !strings.HasPrefix(line, "MAIL") && !strings.HasPrefix(line, "QUIT") && !strings.HasPrefix(line, "*"):
			// a CRAM-MD5 response line
			reply = s.auth2
			authed = false
		case strings.HasPrefix(line, "MAIL"):
			reply = s.mail
		case strings.HasPrefix(line, "RCPT"):
			reply = "250 2.1.5 ok"
			if nrcpt < len(rcpts) && rcpts[nrcpt] != "" {
				reply = rcpts[nrcpt]
			}
			nrcpt++
		case line == "DATA":
			reply = s.data
			if strings.HasPrefix(s.data, "354") {
				inData = true
			}
		case line == "QUIT":
			write(s.quit)
			return seen
		default:
			reply = "500 5.5.2 unknown"
		}
		write(reply)
	}
}

func smtpErrText(err error) string {
	var te *textproto.Error
	if errors.As(err, &te) {
		return fmt.Sprintf("err %d %s", te.Code, te.Msg)
	}
	return "err 0 " + err.Error()
}

// smtpRun is SendMail against the script: the transcript and the result.
func smtpRun(s smtpScript, from string, to []string, msg string, auth int) (string, string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	done := make(chan []string)
	go func() {
		c, err := l.Accept()
		if err != nil {
			done <- nil
			return
		}
		done <- smtpServe(c, s)
	}()
	var a smtp.Auth
	switch auth {
	case 1:
		a = smtp.PlainAuth("", "user", "pw", "127.0.0.1")
	case 2:
		a = smtp.CRAMMD5Auth("user", "pass")
	}
	res := "ok"
	if err := smtp.SendMail(l.Addr().String(), a, from, to, []byte(msg)); err != nil {
		res = smtpErrText(err)
	}
	l.Close()
	var seen []string
	if res == "err 0 smtp: A line must not contain CR or LF" {
		// SendMail refused before dialing: no session
		go func() { done <- nil }()
	}
	seen = <-done
	return strings.Join(seen, "‣"), res
}

func smtpCorpus() string {
	r := rand.New(rand.NewSource(15))
	var o strings.Builder
	o.WriteString(header("net/smtp's SendMail against a scripted server on 127.0.0.1 (greetings, EHLO and the HELO fallback, extensions, AUTH PLAIN and CRAM-MD5, MAIL, RCPT, DATA, QUIT replies, multi-line and malformed), with plain text to loopback"))
	o.WriteString("use std/smtp\n\n/// (greet, ehlo ok, ext, helo, auth, auth2, mail, rcpts, data, end, quit; from, to, message, auth kind; Go's transcript (lines joined by U+2023) and result)\nconst CASES: List[((Str, Bool, Str, Str, Str, Str, Str, Str, Str, Str, Str), (Str, Str, Str, Int), (Str, Str))] = [\n")
	pick := func(ok string, others ...string) string {
		if r.Intn(4) > 0 {
			return ok
		}
		return others[r.Intn(len(others))]
	}
	for i := 0; i < 500; i++ {
		s := smtpScript{
			greet:  pick("220 mail.test ESMTP", "554 5.3.2 no service", "220-mail.test\n220 welcome", "22", "abc", "220-x\n221 y\n220 z"),
			ehloOK: r.Intn(6) > 0,
			ext:    []string{"8BITMIME;AUTH PLAIN CRAM-MD5", "AUTH PLAIN", "", "SIZE 100;8BITMIME;AUTH PLAIN", "AUTH CRAM-MD5;SMTPUTF8", "8BITMIME;AUTH CRAM-MD5 PLAIN"}[r.Intn(6)],
			helo:   pick("250 mail.test", "550 no", "250-a\n250 b"),
			auth:   pick("235 2.7.0 ok", "535 5.7.8 bad credentials", "334 PDEyMzQ1Ni4xMzIyODc2OTE0QHRlc3RzZXJ2ZXI+", "454 4.7.0 later"),
			auth2:  pick("235 2.7.0 ok", "535 5.7.8 no"),
			mail:   pick("250 2.1.0 ok", "451 4.3.0 later", "550-5.1.0 a\n550 5.1.0 b", "250"),
			rcpts:  []string{"", "", "550 5.1.1 no such user", ";452 4.5.3 too many", "251 2.1.5 forwarded", "250-a\n250 b"}[r.Intn(6)],
			data:   pick("354 go ahead", "554 5.5.1 no valid recipients", "250 not 354"),
			end:    pick("250 2.0.0 queued", "552 5.3.4 too big", "250-a\n250 b"),
			quit:   pick("221 2.0.0 bye", "250 not 221", "500 x"),
		}
		if s.auth == "334 PDEyMzQ1Ni4xMzIyODc2OTE0QHRlc3RzZXJ2ZXI+" && r.Intn(2) == 0 {
			s.auth = "334 " + []string{"", "!!notbase64", "YQ=="}[r.Intn(3)]
		}
		from := pick("a@x.test", "", "é@x.test", "a@x.test", "bad\r\nx@y")
		to := []string{pick("b@y.test", "c@y.test")}
		for k, n := 0, r.Intn(3); k < n; k++ {
			to = append(to, pick("d@y.test", "f@y.test", "f@y.test", "e\n@y.test"))
		}
		msg := pick("Subject: hi\n\nbody\n", ".leading\n..two\nend", "a\r\nb\r\n", "", "no newline", "x\n.\ny\n")
		auth := r.Intn(3)
		seen, res := smtpRun(s, from, to, msg, auth)
		fmt.Fprintf(&o, "  ((%s, %v, %s, %s, %s, %s, %s, %s, %s, %s, %s), (%s, %s, %s, %d), (%s, %s)),\n", xq(s.greet), s.ehloOK, xq(s.ext), xq(s.helo), xq(s.auth), xq(s.auth2), xq(s.mail), xq(s.rcpts), xq(s.data), xq(s.end), xq(s.quit), xq(from), xq(strings.Join(to, ";")), xq(msg), auth, xq(seen), xq(res))
	}
	o.WriteString("]\n")
	o.WriteString(smtpTests)
	return o.String()
}

const smtpTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

struct Script {
  greet: Str
  ehlo_ok: Bool
  ext: Str
  helo: Str
  auth: Str
  auth2: Str
  mail: Str
  rcpts: Str
  data: Str
  end: Str
  quit: Str
}

fn reply(c: ServerConn, r: Str) -> () ! NetErr uses net {
  for l in r.split("\n") {
    c.write("${l}\r\n".bytes())?
  }
}

/// The scripted server of scripts/stdcorpus smtp.go's smtpServe: the
/// client's lines go to seen as they come.
fn serve(l: Listener, s: Script, seen: Mutex[List[Str]]) -> () ! NetErr uses net {
  let c = l.accept()?
  let push = fn(x: Str) { seen.with(fn(var v) { v.push(x) }) }
  reply(c, s.greet)?
  let rcpts = s.rcpts.split(";")
  var nrcpt = 0
  var in_data = false
  var data = ""
  var authed = false
  var buf = b""
  while true {
    var i = 0
    while i < buf.len() && buf[i] != '\n' {
      i += 1
    }
    if i == buf.len() {
      match c.read(4096)? {
        Some(b) => {
          buf = buf + b
          continue
        }
        None => break
      }
    }
    let line = (buf[0..i].to_str() ?? "").trim_suffix("\r")
    buf = buf[i + 1..]
    if in_data {
      if line == "." {
        in_data = false
        push("== data")
        push(data)
        reply(c, s.end)?
      } else {
        data = data + line + "|"
      }
      continue
    }
    push(line)
    var r = ""
    if line.starts_with("EHLO") {
      if !s.ehlo_ok {
        r = "502 5.5.1 no EHLO"
      } else {
        var lines = ["mail.test"]
        if !s.ext.is_empty() {
          lines = lines + s.ext.split(";")
        }
        r = lines.indexed().map(fn(p) { "250${if p.0 == lines.len() - 1 { " " } else { "-" }}${p.1}" }).join("\n")
      }
    } else if line.starts_with("HELO") {
      r = s.helo
    } else if line.starts_with("AUTH") {
      r = s.auth
      // a 334 challenge: the next line answers it
      authed = s.auth.starts_with("334")
    } else if authed && !line.starts_with("MAIL") && !line.starts_with("QUIT") && !line.starts_with("*") {
      r = s.auth2
      authed = false
    } else if line.starts_with("MAIL") {
      r = s.mail
    } else if line.starts_with("RCPT") {
      r = if nrcpt < rcpts.len() && !rcpts[nrcpt].is_empty() { rcpts[nrcpt] } else { "250 2.1.5 ok" }
      nrcpt += 1
    } else if line == "DATA" {
      r = s.data
      if s.data.starts_with("354") {
        in_data = true
      }
    } else if line == "QUIT" {
      reply(c, s.quit)?
      break
    } else {
      r = "500 5.5.2 unknown"
    }
    reply(c, r)?
  }
  c.close()
}

test "send talks to a server as Go's SendMail does, with its results" {
  var bad: List[(Int, Str, Str)] = []
  for (i, ((greet, ehlo_ok, ext, helo, auth, auth2, mail, rcpts, data, end, quit), (from, to, msg, kind), (want_seen, want))) in CASES.indexed() {
    let s = Script{greet: greet, ehlo_ok: ehlo_ok, ext: ext, helo: helo, auth: auth, auth2: auth2, mail: mail, rcpts: rcpts, data: data, end: end, quit: quit}
    let addr = "127.0.0.1:${20000 + i}"
    let l = os.net.listen(addr)?
    let seen = Mutex[List[Str]].new([])
    let server = os.tasks.spawn(fn() ! NetErr { serve(l, s, seen) })
    let a = match kind {
      1 => smtp.Auth.Plain(user: "user", password: Secret.new("pw"))
      2 => smtp.Auth.CramMd5(user: "user", secret: Secret.new("pass"))
      _ => smtp.Auth.None
    }
    let got = match smtp.send(os.net, addr, from, to.split(";"), msg.bytes(), auth: a, security: smtp.Security.None) {
      Ok(_) => "ok"
      Err(e) => "err ${e.code} ${e.msg}"
    }
    l.close()
    let _ = server.await.ignore_err("the server ends when the listener closes")
    let got_seen = seen.with(fn(v) { v.join("\u{2023}") })
    if got != want || got_seen != want_seen {
      bad.push((i, "${got} ${got_seen}", "${want} ${want_seen}"))
    }
  }
  expect(first_bad(bad)) == []
}
`
