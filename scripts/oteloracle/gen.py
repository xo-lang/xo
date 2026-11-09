#!/usr/bin/env python3
"""Generates std/native/trace_oracle_test.xo from the OpenTelemetry Go oracle's
out/*.spec and out/*.bin (Xo Azure 1's program, README-oracle.md): the spans of
each case are built as std/trace would, encoded by std/otlp, and compared byte
for byte with Go's request body. Usage: python3 -I scripts/oteloracle/gen.py"""
import json, os, re, shlex
here = os.path.dirname(os.path.abspath(__file__))
out = os.path.join(here, 'out')
KIND = {'unspecified': 'Unspecified', 'internal': 'Internal', 'server': 'Server', 'client': 'Client', 'producer': 'Producer', 'consumer': 'Consumer'}

def q(s):
    return json.dumps(s)

def val(t, v):
    j = json.loads(v)
    if t == 'string':
        return 'trace.AttrValue.Str(%s)' % q(j)
    if t == 'int64':
        n = j
        lit = '-9223372036854775807 - 1' if n == -9223372036854775808 else str(n)
        return 'trace.AttrValue.Int(%s)' % lit
    if t == 'float64':
        s = repr(float(j))
        return 'trace.AttrValue.Float(%s)' % s
    return 'trace.AttrValue.Bool(%s)' % ('true' if j else 'false')

def attr(rest):
    # "key" TYPE VALUE
    m = re.match(r'("(?:[^"\\]|\\.)*") (\w+) (.*)$', rest)
    return 'trace.Attr{key: %s, value: %s}' % (m.group(1), val(m.group(2), m.group(3)))

def attr_json(s):
    m = re.match(r'\s*("(?:[^"\\]|\\.)*") (\w+) (.*)$', s)
    return 'trace.Attr{key: %s, value: %s}' % (m.group(1), val(m.group(2), m.group(3).strip()))

lines = []
cases = sorted(f[:-5] for f in os.listdir(out) if f.endswith('.spec'))
for case in cases:
    spec = open(os.path.join(out, case + '.spec')).read().splitlines()
    want = open(os.path.join(out, case + '.bin'), 'rb').read().hex()
    scopes = {}
    spans = []
    for ln in spec:
        t = ln.strip()
        if ln.startswith('scope '):
            m = re.match(r'scope (\d+) ("(?:[^"\\]|\\.)*") ("(?:[^"\\]|\\.)*") ("(?:[^"\\]|\\.)*")$', ln)
            scopes[int(m.group(1))] = (m.group(2), m.group(3), m.group(4))
        elif ln.startswith('span '):
            m = re.match(r'span (\w+) (\w+) (\w+) (\w+) ("(?:[^"\\]|\\.)*") (\d+) (\d+) scope=(\d+)$', ln)
            spans.append({'tid': m.group(1), 'sid': m.group(2), 'parent': m.group(3), 'kind': m.group(4), 'name': m.group(5),
                          'start': int(m.group(6)), 'end': int(m.group(7)), 'scope': int(m.group(8)), 'ops': []})
        elif t.startswith('attr '):
            spans[-1]['ops'].append(('attr', attr(t[5:])))
        elif t.startswith('set '):
            parts = [p for p in re.split(r'\s\|\s', t[4:] if not t[4:].startswith('|') else t[4:]) if p.strip()]
            body = t[4:].strip()
            assert body.startswith('|')
            items = [x.strip() for x in re.split(r'\s\|\s', body[1:].strip())]
            spans[-1]['ops'].append(('set', [attr_json(x) for x in items]))
        elif t.startswith('event '):
            m = re.match(r'event ("(?:[^"\\]|\\.)*") (\d+)$', t)
            spans[-1]['ops'].append(('event', m.group(1), int(m.group(2)), []))
        elif t.startswith('eattr '):
            spans[-1]['ops'][-1][3].append(attr(t[6:]))
        elif t.startswith('status '):
            m = re.match(r'status (\w+) ("(?:[^"\\]|\\.)*")$', t)
            spans[-1]['ops'].append(('status', m.group(1), m.group(2)))
    # spans end in file order, so the body lists them in file order
    fn = case.replace('-', '_')
    lines.append('test "OpenTelemetry Go oracle: %s" {' % case)
    lines.append('  var spans: List[trace.Span] = []')
    for idx, sp in enumerate(spans):
        v = 's%d' % idx
        sn, sv, sc = scopes[sp['scope']]
        lines.append('  var %s = trace.Span{id: "%s", name: %s, start_ms: %d, micros: %d, status: 0, trace_id: "%s", span_id: "%s", parent_id: "%s", kind: Some(trace.SpanKind.%s), start_ns: %d, end_ns: %d, scope: %s, scope_version: %s, schema_url: %s}'
                     % (v, sp['tid'], sp['name'], sp['start'] // 1000000, (sp['end'] - sp['start']) // 1000, sp['tid'], sp['sid'], sp['parent'], KIND[sp['kind']], sp['start'], sp['end'], sn, sv, sc))
        starts = [o[1] for o in sp['ops'] if o[0] == 'attr']
        if starts:
            lines.append('  %s = %s with {attributes: merge_attrs([], [%s])}' % (v, v, ', '.join(starts)))
        for o in sp['ops']:
            if o[0] == 'set':
                lines.append('  %s = %s with {attributes: merge_attrs(%s.attributes, [%s])}' % (v, v, v, ', '.join(o[1])))
            elif o[0] == 'event':
                ev = 'trace.Event{name: %s, time_ns: %d, attributes: [%s]}' % (o[1], o[2], ', '.join(o[3]))
                lines.append('  %s = %s with {events: list_with(%s.events, %s)}' % (v, v, v, ev))
            elif o[0] == 'status':
                lines.append('  %s = span_set_status(%s, trace.StatusCode.%s, %s)' % (v, v, o[1], o[2]))
        lines.append('  spans.push(%s)' % v)
    lines.append('  let got = otlp.encode_traces(otlp.Resource{service: "oracle", scope: "oracle"}, spans)')
    lines.append('  expect(got.to_hex()) == "%s"' % want)
    lines.append('}')
    lines.append('')
hdr = '''// BEGIN GENERATED by scripts/oteloracle/gen.py from the OpenTelemetry Go oracle's out/ (otel v1.47.0, otlptracehttp v1.47.0;
// Xo Azure 1, decision 0580). Do not edit between the markers: regenerate. Each test builds a case's spans with
// std/trace's own rules (merge_attrs, span_set_status), encodes them with std/otlp and compares the bytes with Go's request body.
fn list_with(l: List[trace.Event], e: trace.Event) -> List[trace.Event] {
  var out = l
  out.push(e)
  out
}

'''
tp = os.path.join(here, '..', '..', 'std', 'native', 'trace_test.xo')
t = open(tp).read()
b = t.find('// BEGIN GENERATED')
if b >= 0:
    t = t[:b].rstrip('\n') + '\n'
block = '\n' + hdr + '\n'.join(lines) + '// END GENERATED\n'
open(tp, 'w').write(t + block)
