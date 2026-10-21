package rt

// std/xml (std.md 34, decision 0314): Go's encoding/xml. Tokens come from
// xml.Decoder (Token or RawToken until EOF), the tree is built from them as
// std/native/xml.xo builds it, and encode writes the tree's tokens with
// xml.Encoder. Outside bytes become Str through textOf (decision 0259).

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

func (e Xml_XmlErr) M_display(*Task) string {
	if e.F_line > 0 {
		return "XML syntax error on line " + strconv.FormatInt(e.F_line, 10) + ": " + e.F_msg
	}
	return e.F_msg
}

func xmlErr(err error) Xml_XmlErr {
	var se *xml.SyntaxError
	if errors.As(err, &se) {
		return Xml_XmlErr{F_line: int64(se.Line), F_msg: textOf(se.Msg)}
	}
	return Xml_XmlErr{F_msg: textOf(err.Error())}
}

func xmlName(n xml.Name) Xml_Name {
	return Xml_Name{F_space: textOf(n.Space), F_local: textOf(n.Local)}
}

func xmlAttrs(as []xml.Attr) List[Xml_Attr] {
	out := make([]Xml_Attr, len(as))
	for i, a := range as {
		out[i] = Xml_Attr{F_name: xmlName(a.Name), F_value: textOf(a.Value)}
	}
	return ListFromSlice(out)
}

func xmlToken(t xml.Token) Xml_Token {
	switch v := t.(type) {
	case xml.StartElement:
		return Xml_Token{Tag: 1, P: Xml_Token_Start{F_name: xmlName(v.Name), F_attrs: xmlAttrs(v.Attr)}}
	case xml.EndElement:
		return Xml_Token{Tag: 2, P: Xml_Token_End{F_name: xmlName(v.Name)}}
	case xml.CharData:
		return Xml_Token{Tag: 3, P: Xml_Token_Text{F_s: textOf(string(v))}}
	case xml.Comment:
		return Xml_Token{Tag: 4, P: Xml_Token_Comment{F_s: textOf(string(v))}}
	case xml.ProcInst:
		return Xml_Token{Tag: 5, P: Xml_Token_ProcInst{F_target: textOf(v.Target), F_inst: textOf(string(v.Inst))}}
	case xml.Directive:
		return Xml_Token{Tag: 6, P: Xml_Token_Directive{F_s: textOf(string(v))}}
	}
	panic("xml: unknown token")
}

func xmlDecoder(text string, strict bool) *xml.Decoder {
	d := xml.NewDecoder(strings.NewReader(text))
	if !strict {
		d.Strict = false
		d.AutoClose = xml.HTMLAutoClose
		d.Entity = xml.HTMLEntity
	}
	return d
}

func xmlRead(next func() (xml.Token, error)) ([]Xml_Token, error) {
	var out []Xml_Token
	for {
		t, err := next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, xmlToken(xml.CopyToken(t)))
	}
}

func Xml_tokens(_ *Task, text string, strict bool) Result[List[Xml_Token], Xml_XmlErr] {
	ts, err := xmlRead(xmlDecoder(text, strict).Token)
	if err != nil {
		return Err[List[Xml_Token]](xmlErr(err))
	}
	return Ok[List[Xml_Token], Xml_XmlErr](ListFromSlice(ts))
}

func Xml_raw_tokens(_ *Task, text string) Result[List[Xml_Token], Xml_XmlErr] {
	ts, err := xmlRead(xmlDecoder(text, true).RawToken)
	if err != nil {
		return Err[List[Xml_Token]](xmlErr(err))
	}
	return Ok[List[Xml_Token], Xml_XmlErr](ListFromSlice(ts))
}

// xmlNode is a node of the tree under construction.
type xmlNode struct {
	el       Xml_Element
	children []Xml_Node
}

func Xml_parse(_ *Task, text string, strict bool) Result[Xml_Element, Xml_XmlErr] {
	ts, err := xmlRead(xmlDecoder(text, strict).Token)
	if err != nil {
		return Err[Xml_Element](xmlErr(err))
	}
	fail := func(msg string) Result[Xml_Element, Xml_XmlErr] {
		return Err[Xml_Element](Xml_XmlErr{F_msg: msg})
	}
	var stack []*xmlNode
	var root *Xml_Element
	for _, t := range ts {
		if t.Tag == 1 {
			if len(stack) == 0 && root != nil {
				return fail("xml: more than one root element")
			}
			s := t.P.(Xml_Token_Start)
			stack = append(stack, &xmlNode{el: Xml_Element{F_name: s.F_name, F_attrs: s.F_attrs}})
			continue
		}
		if t.Tag == 2 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n.el.F_children = ListFromSlice(n.children)
			if len(stack) == 0 {
				e := n.el
				root = &e
			} else {
				p := stack[len(stack)-1]
				p.children = append(p.children, Xml_Node{Tag: 1, P: Xml_Node_Elem{F_e: n.el}})
			}
			continue
		}
		var node Xml_Node
		switch t.Tag {
		case 3:
			s := t.P.(Xml_Token_Text).F_s
			if len(stack) == 0 {
				if strings.TrimLeft(s, " \t\r\n") != "" {
					return fail("xml: text outside the root element")
				}
				continue
			}
			node = Xml_Node{Tag: 2, P: Xml_Node_Text{F_s: s}}
		case 4:
			node = Xml_Node{Tag: 3, P: Xml_Node_Comment{F_s: t.P.(Xml_Token_Comment).F_s}}
		case 5:
			p := t.P.(Xml_Token_ProcInst)
			node = Xml_Node{Tag: 4, P: Xml_Node_ProcInst{F_target: p.F_target, F_inst: p.F_inst}}
		default:
			node = Xml_Node{Tag: 5, P: Xml_Node_Directive{F_s: t.P.(Xml_Token_Directive).F_s}}
		}
		if len(stack) > 0 {
			p := stack[len(stack)-1]
			p.children = append(p.children, node)
		}
	}
	if root == nil {
		return fail("xml: no root element")
	}
	return Ok[Xml_Element, Xml_XmlErr](*root)
}

func (e Xml_Element) M_attr(_ *Task, local string) Option[string] {
	for _, a := range e.F_attrs.s {
		if a.F_name.F_local == local {
			return Some(a.F_value)
		}
	}
	return None[string]()
}

func (e Xml_Element) M_child(_ *Task, local string) Option[Xml_Element] {
	for _, n := range e.F_children.s {
		if n.Tag == 1 && n.P.(Xml_Node_Elem).F_e.F_name.F_local == local {
			return Some(n.P.(Xml_Node_Elem).F_e)
		}
	}
	return None[Xml_Element]()
}

func (e Xml_Element) M_children_named(_ *Task, local string) List[Xml_Element] {
	var out []Xml_Element
	for _, n := range e.F_children.s {
		if n.Tag == 1 && n.P.(Xml_Node_Elem).F_e.F_name.F_local == local {
			out = append(out, n.P.(Xml_Node_Elem).F_e)
		}
	}
	return ListFromSlice(out)
}

func (e Xml_Element) xmlText(b *strings.Builder) {
	for _, n := range e.F_children.s {
		switch n.Tag {
		case 1:
			n.P.(Xml_Node_Elem).F_e.xmlText(b)
		case 2:
			b.WriteString(n.P.(Xml_Node_Text).F_s)
		}
	}
}

func (e Xml_Element) M_text(*Task) string {
	var b strings.Builder
	e.xmlText(&b)
	return b.String()
}

func xmlGoName(n Xml_Name) xml.Name { return xml.Name{Space: n.F_space, Local: n.F_local} }

func (e Xml_Element) xmlEncode(enc *xml.Encoder, w *bytes.Buffer) error {
	start := xml.StartElement{Name: xmlGoName(e.F_name)}
	for _, a := range e.F_attrs.s {
		start.Attr = append(start.Attr, xml.Attr{Name: xmlGoName(a.F_name), Value: a.F_value})
	}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, n := range e.F_children.s {
		var err error
		switch n.Tag {
		case 1:
			err = n.P.(Xml_Node_Elem).F_e.xmlEncode(enc, w)
		case 2:
			err = enc.EncodeToken(xml.CharData(n.P.(Xml_Node_Text).F_s))
		case 3:
			err = enc.EncodeToken(xml.Comment(n.P.(Xml_Node_Comment).F_s))
		case 4:
			p := n.P.(Xml_Node_ProcInst)
			err = enc.EncodeToken(xml.ProcInst{Target: p.F_target, Inst: []byte(p.F_inst)})
		case 6:
			if err = enc.Flush(); err == nil {
				w.WriteString(n.P.(Xml_Node_Raw).F_s)
			}
		default:
			err = enc.EncodeToken(xml.Directive(n.P.(Xml_Node_Directive).F_s))
		}
		if err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

func (e Xml_Element) M_encode(_ *Task, indent string) Result[string, Xml_XmlErr] {
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	if indent != "" {
		enc.Indent("", indent)
	}
	if err := e.xmlEncode(enc, &b); err != nil {
		return Err[string](xmlErr(err))
	}
	if err := enc.Flush(); err != nil {
		return Err[string](xmlErr(err))
	}
	return Ok[string, Xml_XmlErr](textOf(b.String()))
}

func Xml_escape(_ *Task, s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
