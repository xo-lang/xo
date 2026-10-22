// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rt

import (
	"strconv"
	"sync"
)

// encoding/json's scanner (scanner.go of go1.26), copied (BSD,
// THIRD_PARTY_NOTICES) with its names prefixed js, so that a text that is
// not valid JSON is checked once and its error is the one Go's Unmarshal
// gives, without the second scan and the allocations of calling Unmarshal
// for the error (bench/results/json-invalid-go-backend-2026-10-22.md).

// A jsScanner is a JSON scanning state machine.
// Callers call scan.reset and then pass bytes in one at a time
// by calling scan.step(&scan, c) for each byte.
// The return value, referred to as an opcode, tells the
// caller about significant parsing events like beginning
// and ending literals, objects, and arrays, so that the
// caller can follow along if it wishes.
// The return value jsScanEnd indicates that a single top-level
// JSON value has been completed, *before* the byte that
// just got passed in.  (The indication must be delayed in order
// to recognize the end of numbers: is 123 a whole value or
// the beginning of 12345e+6?).
type jsScanner struct {
	// The step is a func to be called to execute the next transition.
	// Also tried using an integer constant and a single func
	// with a switch, but using the func directly was 10% faster
	// on a 64-bit Mac Mini, and it's nicer to read.
	step func(*jsScanner, byte) int

	// Reached end of top-level value.
	endTop bool

	// Stack of what we're in the middle of - array values, object keys, object values.
	jsParseState []int

	// Error that happened, if any.
	err *jsSyntaxError

	// total bytes consumed, updated by decoder.Decode (and deliberately
	// not set to zero by scan.reset)
	bytes int64
}

var jsScannerPool = sync.Pool{
	New: func() any {
		return &jsScanner{}
	},
}

func jsNewScanner() *jsScanner {
	scan := jsScannerPool.Get().(*jsScanner)
	// scan.reset by design doesn't set bytes to zero
	scan.bytes = 0
	scan.reset()
	return scan
}

func jsFreeScanner(scan *jsScanner) {
	// Avoid hanging on to too much memory in extreme cases.
	if len(scan.jsParseState) > 1024 {
		scan.jsParseState = nil
	}
	jsScannerPool.Put(scan)
}

// These values are returned by the state transition functions
// assigned to jsScanner.state and the method jsScanner.eof.
// They give details about the current state of the scan that
// callers might be interested to know about.
// It is okay to ignore the return value of any particular
// call to jsScanner.state: if one call returns jsScanError,
// every subsequent call will return jsScanError too.
const (
	// Continue.
	jsScanContinue     = iota // uninteresting byte
	jsScanBeginLiteral        // end implied by next result != jsScanContinue
	jsScanBeginObject         // begin object
	jsScanObjectKey           // just finished object key (string)
	jsScanObjectValue         // just finished non-last object value
	jsScanEndObject           // end object (implies jsScanObjectValue if possible)
	jsScanBeginArray          // begin array
	jsScanArrayValue          // just finished array value
	jsScanEndArray            // end array (implies jsScanArrayValue if possible)
	jsScanSkipSpace           // space byte; can skip; known to be last "continue" result

	// Stop.
	jsScanEnd   // top-level value ended *before* this byte; known to be first "stop" result
	jsScanError // hit an error, jsScanner.err.
)

// These values are stored in the jsParseState stack.
// They give the current state of a composite value
// being scanned. If the parser is inside a nested value
// the jsParseState describes the nested state, outermost at entry 0.
const (
	jsParseObjectKey   = iota // parsing object key (before colon)
	jsParseObjectValue        // parsing object value (after colon)
	jsParseArrayValue         // parsing array value
)

// This limits the max nesting depth to prevent stack overflow.
// This is permitted by https://tools.ietf.org/html/rfc7159#section-9
const jsMaxNestingDepth = 10000

// reset prepares the jsScanner for use.
// It must be called before calling s.step.
func (s *jsScanner) reset() {
	s.step = jsStateBeginValue
	s.jsParseState = s.jsParseState[0:0]
	s.err = nil
	s.endTop = false
}

// eof tells the jsScanner that the end of input has been reached.
// It returns a scan status just as s.step does.
func (s *jsScanner) eof() int {
	if s.err != nil {
		return jsScanError
	}
	if s.endTop {
		return jsScanEnd
	}
	s.step(s, ' ')
	if s.endTop {
		return jsScanEnd
	}
	if s.err == nil {
		s.err = &jsSyntaxError{"unexpected end of JSON input", s.bytes}
	}
	return jsScanError
}

// pushParseState pushes a new parse state newParseState onto the parse stack.
// an error state is returned if jsMaxNestingDepth was exceeded, otherwise successState is returned.
func (s *jsScanner) pushParseState(c byte, newParseState int, successState int) int {
	s.jsParseState = append(s.jsParseState, newParseState)
	if len(s.jsParseState) <= jsMaxNestingDepth {
		return successState
	}
	return s.error(c, "exceeded max depth")
}

// popParseState pops a parse state (already obtained) off the stack
// and updates s.step accordingly.
func (s *jsScanner) popParseState() {
	n := len(s.jsParseState) - 1
	s.jsParseState = s.jsParseState[0:n]
	if n == 0 {
		s.step = jsStateEndTop
		s.endTop = true
	} else {
		s.step = jsStateEndValue
	}
}

func jsIsSpace(c byte) bool {
	return c <= ' ' && (c == ' ' || c == '\t' || c == '\r' || c == '\n')
}

// jsStateBeginValueOrEmpty is the state after reading `[`.
func jsStateBeginValueOrEmpty(s *jsScanner, c byte) int {
	if jsIsSpace(c) {
		return jsScanSkipSpace
	}
	if c == ']' {
		return jsStateEndValue(s, c)
	}
	return jsStateBeginValue(s, c)
}

// jsStateBeginValue is the state at the beginning of the input.
func jsStateBeginValue(s *jsScanner, c byte) int {
	if jsIsSpace(c) {
		return jsScanSkipSpace
	}
	switch c {
	case '{':
		s.step = jsStateBeginStringOrEmpty
		return s.pushParseState(c, jsParseObjectKey, jsScanBeginObject)
	case '[':
		s.step = jsStateBeginValueOrEmpty
		return s.pushParseState(c, jsParseArrayValue, jsScanBeginArray)
	case '"':
		s.step = jsStateInString
		return jsScanBeginLiteral
	case '-':
		s.step = jsStateNeg
		return jsScanBeginLiteral
	case '0': // beginning of 0.123
		s.step = jsState0
		return jsScanBeginLiteral
	case 't': // beginning of true
		s.step = jsStateT
		return jsScanBeginLiteral
	case 'f': // beginning of false
		s.step = jsStateF
		return jsScanBeginLiteral
	case 'n': // beginning of null
		s.step = jsStateN
		return jsScanBeginLiteral
	}
	if '1' <= c && c <= '9' { // beginning of 1234.5
		s.step = jsState1
		return jsScanBeginLiteral
	}
	return s.error(c, "looking for beginning of value")
}

// jsStateBeginStringOrEmpty is the state after reading `{`.
func jsStateBeginStringOrEmpty(s *jsScanner, c byte) int {
	if jsIsSpace(c) {
		return jsScanSkipSpace
	}
	if c == '}' {
		n := len(s.jsParseState)
		s.jsParseState[n-1] = jsParseObjectValue
		return jsStateEndValue(s, c)
	}
	return jsStateBeginString(s, c)
}

// jsStateBeginString is the state after reading `{"key": value,`.
func jsStateBeginString(s *jsScanner, c byte) int {
	if jsIsSpace(c) {
		return jsScanSkipSpace
	}
	if c == '"' {
		s.step = jsStateInString
		return jsScanBeginLiteral
	}
	return s.error(c, "looking for beginning of object key string")
}

// jsStateEndValue is the state after completing a value,
// such as after reading `{}` or `true` or `["x"`.
func jsStateEndValue(s *jsScanner, c byte) int {
	n := len(s.jsParseState)
	if n == 0 {
		// Completed top-level before the current byte.
		s.step = jsStateEndTop
		s.endTop = true
		return jsStateEndTop(s, c)
	}
	if jsIsSpace(c) {
		s.step = jsStateEndValue
		return jsScanSkipSpace
	}
	ps := s.jsParseState[n-1]
	switch ps {
	case jsParseObjectKey:
		if c == ':' {
			s.jsParseState[n-1] = jsParseObjectValue
			s.step = jsStateBeginValue
			return jsScanObjectKey
		}
		return s.error(c, "after object key")
	case jsParseObjectValue:
		if c == ',' {
			s.jsParseState[n-1] = jsParseObjectKey
			s.step = jsStateBeginString
			return jsScanObjectValue
		}
		if c == '}' {
			s.popParseState()
			return jsScanEndObject
		}
		return s.error(c, "after object key:value pair")
	case jsParseArrayValue:
		if c == ',' {
			s.step = jsStateBeginValue
			return jsScanArrayValue
		}
		if c == ']' {
			s.popParseState()
			return jsScanEndArray
		}
		return s.error(c, "after array element")
	}
	return s.error(c, "")
}

// jsStateEndTop is the state after finishing the top-level value,
// such as after reading `{}` or `[1,2,3]`.
// Only space characters should be seen now.
func jsStateEndTop(s *jsScanner, c byte) int {
	if !jsIsSpace(c) {
		// Complain about non-space byte on next call.
		s.error(c, "after top-level value")
	}
	return jsScanEnd
}

// jsStateInString is the state after reading `"`.
func jsStateInString(s *jsScanner, c byte) int {
	if c == '"' {
		s.step = jsStateEndValue
		return jsScanContinue
	}
	if c == '\\' {
		s.step = jsStateInStringEsc
		return jsScanContinue
	}
	if c < 0x20 {
		return s.error(c, "in string literal")
	}
	return jsScanContinue
}

// jsStateInStringEsc is the state after reading `"\` during a quoted string.
func jsStateInStringEsc(s *jsScanner, c byte) int {
	switch c {
	case 'b', 'f', 'n', 'r', 't', '\\', '/', '"':
		s.step = jsStateInString
		return jsScanContinue
	case 'u':
		s.step = jsStateInStringEscU
		return jsScanContinue
	}
	return s.error(c, "in string escape code")
}

// jsStateInStringEscU is the state after reading `"\u` during a quoted string.
func jsStateInStringEscU(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' {
		s.step = jsStateInStringEscU1
		return jsScanContinue
	}
	// numbers
	return s.error(c, "in \\u hexadecimal character escape")
}

// jsStateInStringEscU1 is the state after reading `"\u1` during a quoted string.
func jsStateInStringEscU1(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' {
		s.step = jsStateInStringEscU12
		return jsScanContinue
	}
	// numbers
	return s.error(c, "in \\u hexadecimal character escape")
}

// jsStateInStringEscU12 is the state after reading `"\u12` during a quoted string.
func jsStateInStringEscU12(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' {
		s.step = jsStateInStringEscU123
		return jsScanContinue
	}
	// numbers
	return s.error(c, "in \\u hexadecimal character escape")
}

// jsStateInStringEscU123 is the state after reading `"\u123` during a quoted string.
func jsStateInStringEscU123(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' {
		s.step = jsStateInString
		return jsScanContinue
	}
	// numbers
	return s.error(c, "in \\u hexadecimal character escape")
}

// jsStateNeg is the state after reading `-` during a number.
func jsStateNeg(s *jsScanner, c byte) int {
	if c == '0' {
		s.step = jsState0
		return jsScanContinue
	}
	if '1' <= c && c <= '9' {
		s.step = jsState1
		return jsScanContinue
	}
	return s.error(c, "in numeric literal")
}

// jsState1 is the state after reading a non-zero integer during a number,
// such as after reading `1` or `100` but not `0`.
func jsState1(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' {
		s.step = jsState1
		return jsScanContinue
	}
	return jsState0(s, c)
}

// jsState0 is the state after reading `0` during a number.
func jsState0(s *jsScanner, c byte) int {
	if c == '.' {
		s.step = jsStateDot
		return jsScanContinue
	}
	if c == 'e' || c == 'E' {
		s.step = jsStateE
		return jsScanContinue
	}
	return jsStateEndValue(s, c)
}

// jsStateDot is the state after reading the integer and decimal point in a number,
// such as after reading `1.`.
func jsStateDot(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' {
		s.step = jsStateDot0
		return jsScanContinue
	}
	return s.error(c, "after decimal point in numeric literal")
}

// jsStateDot0 is the state after reading the integer, decimal point, and subsequent
// digits of a number, such as after reading `3.14`.
func jsStateDot0(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' {
		return jsScanContinue
	}
	if c == 'e' || c == 'E' {
		s.step = jsStateE
		return jsScanContinue
	}
	return jsStateEndValue(s, c)
}

// jsStateE is the state after reading the mantissa and e in a number,
// such as after reading `314e` or `0.314e`.
func jsStateE(s *jsScanner, c byte) int {
	if c == '+' || c == '-' {
		s.step = jsStateESign
		return jsScanContinue
	}
	return jsStateESign(s, c)
}

// jsStateESign is the state after reading the mantissa, e, and sign in a number,
// such as after reading `314e-` or `0.314e+`.
func jsStateESign(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' {
		s.step = jsStateE0
		return jsScanContinue
	}
	return s.error(c, "in exponent of numeric literal")
}

// jsStateE0 is the state after reading the mantissa, e, optional sign,
// and at least one digit of the exponent in a number,
// such as after reading `314e-2` or `0.314e+1` or `3.14e0`.
func jsStateE0(s *jsScanner, c byte) int {
	if '0' <= c && c <= '9' {
		return jsScanContinue
	}
	return jsStateEndValue(s, c)
}

// jsStateT is the state after reading `t`.
func jsStateT(s *jsScanner, c byte) int {
	if c == 'r' {
		s.step = jsStateTr
		return jsScanContinue
	}
	return s.error(c, "in literal true (expecting 'r')")
}

// jsStateTr is the state after reading `tr`.
func jsStateTr(s *jsScanner, c byte) int {
	if c == 'u' {
		s.step = jsStateTru
		return jsScanContinue
	}
	return s.error(c, "in literal true (expecting 'u')")
}

// jsStateTru is the state after reading `tru`.
func jsStateTru(s *jsScanner, c byte) int {
	if c == 'e' {
		s.step = jsStateEndValue
		return jsScanContinue
	}
	return s.error(c, "in literal true (expecting 'e')")
}

// jsStateF is the state after reading `f`.
func jsStateF(s *jsScanner, c byte) int {
	if c == 'a' {
		s.step = jsStateFa
		return jsScanContinue
	}
	return s.error(c, "in literal false (expecting 'a')")
}

// jsStateFa is the state after reading `fa`.
func jsStateFa(s *jsScanner, c byte) int {
	if c == 'l' {
		s.step = jsStateFal
		return jsScanContinue
	}
	return s.error(c, "in literal false (expecting 'l')")
}

// jsStateFal is the state after reading `fal`.
func jsStateFal(s *jsScanner, c byte) int {
	if c == 's' {
		s.step = jsStateFals
		return jsScanContinue
	}
	return s.error(c, "in literal false (expecting 's')")
}

// jsStateFals is the state after reading `fals`.
func jsStateFals(s *jsScanner, c byte) int {
	if c == 'e' {
		s.step = jsStateEndValue
		return jsScanContinue
	}
	return s.error(c, "in literal false (expecting 'e')")
}

// jsStateN is the state after reading `n`.
func jsStateN(s *jsScanner, c byte) int {
	if c == 'u' {
		s.step = jsStateNu
		return jsScanContinue
	}
	return s.error(c, "in literal null (expecting 'u')")
}

// jsStateNu is the state after reading `nu`.
func jsStateNu(s *jsScanner, c byte) int {
	if c == 'l' {
		s.step = jsStateNul
		return jsScanContinue
	}
	return s.error(c, "in literal null (expecting 'l')")
}

// jsStateNul is the state after reading `nul`.
func jsStateNul(s *jsScanner, c byte) int {
	if c == 'l' {
		s.step = jsStateEndValue
		return jsScanContinue
	}
	return s.error(c, "in literal null (expecting 'l')")
}

// jsStateError is the state after reaching a syntax error,
// such as after reading `[1}` or `5.1.2`.
func jsStateError(s *jsScanner, c byte) int {
	return jsScanError
}

// error records an error and switches to the error state.
func (s *jsScanner) error(c byte, context string) int {
	s.step = jsStateError
	s.err = &jsSyntaxError{"invalid character " + jsQuoteChar(c) + " " + context, s.bytes}
	return jsScanError
}

// jsQuoteChar formats c as a quoted character literal.
func jsQuoteChar(c byte) string {
	// special cases - different from quoted strings
	if c == '\'' {
		return `'\''`
	}
	if c == '"' {
		return `'"'`
	}

	// use quoted string with different quotation marks
	s := strconv.Quote(string(c))
	return "'" + s[1:len(s)-1] + "'"
}

// jsCheck is encoding/json's checkValid: nil when data is valid JSON, else
// the scanner's error.
func jsCheck(data []byte) *jsSyntaxError {
	scan := jsNewScanner()
	defer jsFreeScanner(scan)
	scan.reset()
	for _, c := range data {
		scan.bytes++
		if scan.step(scan, c) == jsScanError {
			return scan.err
		}
	}
	if scan.eof() == jsScanError {
		return scan.err
	}
	return nil
}

// A jsSyntaxError is encoding/json's SyntaxError: a description of a JSON
// syntax error, and the offset it occurred after reading.
type jsSyntaxError struct {
	msg    string
	Offset int64
}

func (e *jsSyntaxError) Error() string { return e.msg }
