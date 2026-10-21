package rt

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// std/mail (std.md 25, decision 0282): Go's net/mail addresses, as
// std/native/mail.xo ports them.

func (e Mail_MailErr) M_display(*Task) string { return e.F_msg }

func (a Mail_Address) M_display(*Task) string {
	return (&mail.Address{Name: a.F_name, Address: a.F_address}).String()
}

func Mail_parse_address(_ *Task, s string) Result[Mail_Address, Mail_MailErr] {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return Err[Mail_Address](Mail_MailErr{F_msg: err.Error()})
	}
	return Ok[Mail_Address, Mail_MailErr](Mail_Address{F_name: textOf(a.Name), F_address: textOf(a.Address)})
}

func Mail_parse_address_list(_ *Task, s string) Result[List[Mail_Address], Mail_MailErr] {
	as, err := mail.ParseAddressList(s)
	if err != nil {
		return Err[List[Mail_Address]](Mail_MailErr{F_msg: err.Error()})
	}
	out := make([]Mail_Address, len(as))
	for i, a := range as {
		out[i] = Mail_Address{F_name: textOf(a.Name), F_address: textOf(a.Address)}
	}
	return Ok[List[Mail_Address], Mail_MailErr](ListFromSlice(out))
}

// Messages (decision 0290): message.go ReadMessage and its readHeader
// on net/textproto's Reader, with the fields kept in order.

func Mail_read_message(_ *Task, msg Bytes) Result[Mail_Message, Mail_MailErr] {
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader([]byte(msg))))
	var hs []Tuple2[string, string]
	if len(msg) > 0 && (msg[0] == ' ' || msg[0] == '\t') {
		line, _ := r.ReadLine()
		return mailFail[Mail_Message]("malformed initial line: " + textOf(line))
	}
	for {
		kv, err := r.ReadContinuedLine()
		if kv == "" {
			if err != nil && len(hs) == 0 {
				return mailFail[Mail_Message](err.Error())
			}
			break
		}
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			return mailFail[Mail_Message]("malformed header line: " + textOf(kv))
		}
		key := textproto.CanonicalMIMEHeaderKey(k)
		if key == "" {
			continue
		}
		hs = append(hs, Tuple2[string, string]{textOf(key), textOf(strings.TrimLeft(v, " \t"))})
	}
	body, _ := io.ReadAll(r.R)
	return Ok[Mail_Message, Mail_MailErr](Mail_Message{F_headers: ListFromSlice(hs), F_body: Bytes(body)})
}

func mailFail[T any](msg string) Result[T, Mail_MailErr] { return Err[T](Mail_MailErr{F_msg: msg}) }

func (m Mail_Message) M_header(_ *Task, name string) Option[string] {
	want := textproto.CanonicalMIMEHeaderKey(name)
	for _, h := range m.F_headers.s {
		if h.F0 == want {
			return Some(h.F1)
		}
	}
	return Option[string]{}
}

// mailDateLayouts is message.go's dateLayouts.
var mailDateLayouts = func() []string {
	var out []string
	for _, dow := range []string{"", "Mon, "} {
		for _, day := range []string{"2", "02"} {
			for _, year := range []string{"2006", "06"} {
				for _, sec := range []string{":05", ""} {
					for _, zone := range []string{"-0700", "MST", "UT"} {
						out = append(out, dow+day+" Jan "+year+" 15:04"+sec+" "+zone)
					}
				}
			}
		}
	}
	return out
}()

// mailParseDate is mail.ParseDate with its layouts parsed in UTC
// (time.ParseInLocation): a zone name has offset 0 but UTC and GMT+h, on
// every machine, where time.Parse would take the local zone's offset for
// the local zone's names.
func mailParseDate(date string) (time.Time, string) {
	date = strings.ReplaceAll(date, "\r\n", "")
	if strings.Contains(date, "\r") {
		return time.Time{}, "mail: header has a CR without LF"
	}
	s := strings.TrimLeft(date, " \t")
	if ind := strings.IndexAny(s, "+-"); ind != -1 && len(s) >= ind+5 {
		date, s = s[:ind+5], s[ind+5:]
	} else {
		ind := strings.Index(s, "T")
		if ind == 0 {
			ind = strings.Index(s[1:], "T")
			if ind != -1 {
				ind++
			}
		}
		if ind != -1 && len(s) >= ind+5 {
			date, s = s[:ind+1], s[ind+1:]
		}
	}
	if !mailSkipCFWS(s) {
		return time.Time{}, "mail: misformatted parenthetical comment"
	}
	for _, layout := range mailDateLayouts {
		if t, err := time.ParseInLocation(layout, date, time.UTC); err == nil {
			return t, ""
		}
	}
	return time.Time{}, "mail: header could not be parsed"
}

// mailSkipCFWS is addrParser.skipCFWS on s: false for an unclosed comment.
func mailSkipCFWS(s string) bool {
	s = strings.TrimLeft(s, " \t")
	for strings.HasPrefix(s, "(") {
		s = s[1:]
		depth := 1
		for len(s) > 0 && depth != 0 {
			if s[0] == '\\' && len(s) > 1 {
				s = s[1:]
			} else if s[0] == '(' {
				depth++
			} else if s[0] == ')' {
				depth--
			}
			s = s[1:]
		}
		if depth != 0 {
			return false
		}
		s = strings.TrimLeft(s, " \t")
	}
	return true
}

func Mail_parse_date(_ *Task, s string) Result[Time, Mail_MailErr] {
	t, msg := mailParseDate(s)
	if msg != "" {
		return mailFail[Time](msg)
	}
	if u := t.Unix(); u < -9223372036 || u > 9223372036 {
		return mailFail[Time]("mail: date out of range")
	}
	return Ok[Time, Mail_MailErr](Time(t.UnixNano()))
}

func mailQP(s string) string {
	var b strings.Builder
	w := quotedprintable.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.String()
}

func Mail_compose(tk *Task, from Mail_Address, to List[Mail_Address], subject string, text string, html Option[string], date Time, id string) Bytes {
	var b strings.Builder
	b.WriteString("From: " + from.M_display(tk) + "\r\n")
	tos := make([]string, len(to.s))
	for i, a := range to.s {
		tos[i] = a.M_display(tk)
	}
	b.WriteString("To: " + strings.Join(tos, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Unix(0, int64(date)).UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + id + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	if !html.Some {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(mailQP(text))
		return Bytes(b.String())
	}
	sum := sha256.Sum256([]byte(id))
	boundary := "xo" + hex.EncodeToString(sum[:])[:28]
	b.WriteString("Content-Type: multipart/alternative; boundary=" + strconv.Quote(boundary) + "\r\n\r\n")
	b.WriteString("--" + boundary + "\r\nContent-Transfer-Encoding: quoted-printable\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(mailQP(text))
	b.WriteString("\r\n--" + boundary + "\r\nContent-Transfer-Encoding: quoted-printable\r\nContent-Type: text/html; charset=utf-8\r\n\r\n")
	b.WriteString(mailQP(html.V))
	b.WriteString("\r\n--" + boundary + "--\r\n")
	return Bytes(b.String())
}
