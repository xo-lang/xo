// Command otlpref is the differential oracle for std/otlp (decision 0539): the opentelemetry-proto trace messages
// (ExportTraceServiceRequest and what it holds, field numbers from opentelemetry/proto/trace/v1/trace.proto,
// common/v1/common.proto and resource/v1/resource.proto) built as dynamic messages by google.golang.org/protobuf and
// marshaled by it, over the Resource and spans scripts/stdcorpus sends. One JSON line in per case, one hex line out.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type span struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	StartMs int64  `json:"start_ms"`
	Micros  int64  `json:"micros"`
	Status  int64  `json:"status"`
}

type point struct {
	Values []string  `json:"values"`
	Obs    []float64 `json:"obs"`
}

type family struct {
	Kind   int       `json:"kind"`
	Name   string    `json:"name"`
	Help   string    `json:"help"`
	Labels []string  `json:"labels"`
	Bounds []float64 `json:"bounds"`
	Series []point   `json:"series"`
}

type input struct {
	Families []family    `json:"families"`
	StartMs  int64       `json:"start_ms"`
	NowMs    int64       `json:"now_ms"`
	Service  string      `json:"service"`
	Attrs    [][2]string `json:"attrs"`
	Scope    string      `json:"scope"`
	Version  string      `json:"version"`
	Spans    []span      `json:"spans"`
}

type D = descriptorpb.FieldDescriptorProto

func fld(name string, num int32, t descriptorpb.FieldDescriptorProto_Type, rep bool, tn string) *D {
	l := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	if rep {
		l = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	d := &D{Name: proto.String(name), Number: proto.Int32(num), Type: t.Enum(), Label: l.Enum(), JsonName: proto.String(name)}
	if tn != "" {
		d.TypeName = proto.String(tn)
	}
	return d
}

func msg(name string, fs ...*D) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{Name: proto.String(name), Field: fs}
}

const (
	tStr   = descriptorpb.FieldDescriptorProto_TYPE_STRING
	tBytes = descriptorpb.FieldDescriptorProto_TYPE_BYTES
	tMsg   = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	tI64   = descriptorpb.FieldDescriptorProto_TYPE_INT64
	tF64   = descriptorpb.FieldDescriptorProto_TYPE_FIXED64
	tI32   = descriptorpb.FieldDescriptorProto_TYPE_INT32
	tEnum  = descriptorpb.FieldDescriptorProto_TYPE_ENUM
	tBool  = descriptorpb.FieldDescriptorProto_TYPE_BOOL
	tDbl   = descriptorpb.FieldDescriptorProto_TYPE_DOUBLE
)

func main() {
	anyValue := msg("AnyValue", fld("string_value", 1, tStr, false, ""), fld("int_value", 3, tI64, false, ""))
	anyValue.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("value")}}
	for _, f := range anyValue.Field {
		f.OneofIndex = proto.Int32(0)
	}
	status := msg("Status", fld("message", 2, tStr, false, ""), fld("code", 3, tEnum, false, ".o.StatusCode"))
	spanM := msg("Span", fld("trace_id", 1, tBytes, false, ""), fld("span_id", 2, tBytes, false, ""),
		fld("parent_span_id", 4, tBytes, false, ""), fld("name", 5, tStr, false, ""), fld("kind", 6, tEnum, false, ".o.SpanKind"),
		fld("start_time_unix_nano", 7, tF64, false, ""), fld("end_time_unix_nano", 8, tF64, false, ""),
		fld("attributes", 9, tMsg, true, ".o.KeyValue"), fld("status", 15, tMsg, false, ".o.Status"))
	numberPoint := msg("NumberDataPoint", fld("start_time_unix_nano", 2, tF64, false, ""), fld("time_unix_nano", 3, tF64, false, ""),
		fld("as_double", 4, tDbl, false, ""), fld("attributes", 7, tMsg, true, ".o.KeyValue"))
	numberPoint.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("value")}}
	numberPoint.Field[2].OneofIndex = proto.Int32(0)
	histPoint := msg("HistogramDataPoint", fld("start_time_unix_nano", 2, tF64, false, ""), fld("time_unix_nano", 3, tF64, false, ""),
		fld("count", 4, tF64, false, ""), fld("sum", 5, tDbl, false, ""), fld("bucket_counts", 6, tF64, true, ""),
		fld("explicit_bounds", 7, tDbl, true, ""), fld("attributes", 9, tMsg, true, ".o.KeyValue"))
	histPoint.Field[3].Proto3Optional = proto.Bool(true)
	histPoint.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("_sum")}}
	histPoint.Field[3].OneofIndex = proto.Int32(0)
	summPoint := msg("SummaryDataPoint", fld("start_time_unix_nano", 2, tF64, false, ""), fld("time_unix_nano", 3, tF64, false, ""),
		fld("count", 4, tF64, false, ""), fld("sum", 5, tDbl, false, ""), fld("attributes", 7, tMsg, true, ".o.KeyValue"))
	metricM := msg("Metric", fld("name", 1, tStr, false, ""), fld("description", 2, tStr, false, ""),
		fld("gauge", 5, tMsg, false, ".o.Gauge"), fld("sum", 7, tMsg, false, ".o.Sum"), fld("histogram", 9, tMsg, false, ".o.Histogram"), fld("summary", 11, tMsg, false, ".o.Summary"))
	metricM.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("data")}}
	for _, i := range []int{2, 3, 4, 5} {
		metricM.Field[i].OneofIndex = proto.Int32(0)
	}
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("otlp.proto"), Syntax: proto.String("proto3"), Package: proto.String("o"),
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{Name: proto.String("StatusCode"), Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("UNSET"), Number: proto.Int32(0)}, {Name: proto.String("OK"), Number: proto.Int32(1)}, {Name: proto.String("ERROR"), Number: proto.Int32(2)}}},
			{Name: proto.String("Temporality"), Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("UNSPEC"), Number: proto.Int32(0)}, {Name: proto.String("DELTA"), Number: proto.Int32(1)}, {Name: proto.String("CUMULATIVE"), Number: proto.Int32(2)}}},
			{Name: proto.String("SpanKind"), Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("UNSPECIFIED"), Number: proto.Int32(0)}, {Name: proto.String("INTERNAL"), Number: proto.Int32(1)}, {Name: proto.String("SERVER"), Number: proto.Int32(2)}}},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			anyValue,
			msg("KeyValue", fld("key", 1, tStr, false, ""), fld("value", 2, tMsg, false, ".o.AnyValue")),
			msg("InstrumentationScope", fld("name", 1, tStr, false, ""), fld("version", 2, tStr, false, "")),
			msg("Resource", fld("attributes", 1, tMsg, true, ".o.KeyValue")),
			status, spanM,
			msg("ScopeSpans", fld("scope", 1, tMsg, false, ".o.InstrumentationScope"), fld("spans", 2, tMsg, true, ".o.Span")),
			msg("ResourceSpans", fld("resource", 1, tMsg, false, ".o.Resource"), fld("scope_spans", 2, tMsg, true, ".o.ScopeSpans")),
			msg("ExportTraceServiceRequest", fld("resource_spans", 1, tMsg, true, ".o.ResourceSpans")),
			numberPoint, histPoint, summPoint,
			msg("Gauge", fld("data_points", 1, tMsg, true, ".o.NumberDataPoint")),
			msg("Sum", fld("data_points", 1, tMsg, true, ".o.NumberDataPoint"), fld("aggregation_temporality", 2, tEnum, false, ".o.Temporality"), fld("is_monotonic", 3, tBool, false, "")),
			msg("Histogram", fld("data_points", 1, tMsg, true, ".o.HistogramDataPoint"), fld("aggregation_temporality", 2, tEnum, false, ".o.Temporality")),
			msg("Summary", fld("data_points", 1, tMsg, true, ".o.SummaryDataPoint")),
			metricM,
			msg("ScopeMetrics", fld("scope", 1, tMsg, false, ".o.InstrumentationScope"), fld("metrics", 2, tMsg, true, ".o.Metric")),
			msg("ResourceMetrics", fld("resource", 1, tMsg, false, ".o.Resource"), fld("scope_metrics", 2, tMsg, true, ".o.ScopeMetrics")),
			msg("ExportMetricsServiceRequest", fld("resource_metrics", 1, tMsg, true, ".o.ResourceMetrics")),
		},
	}, nil)
	if err != nil {
		panic(err)
	}
	md := func(n string) protoreflect.MessageDescriptor { return file.Messages().ByName(protoreflect.Name(n)) }
	kv := func(k string, str *string, i *int64) protoreflect.Message {
		m := dynamicpb.NewMessage(md("KeyValue")).ProtoReflect()
		if k != "" {
			m.Set(m.Descriptor().Fields().ByName("key"), protoreflect.ValueOfString(k))
		}
		v := dynamicpb.NewMessage(md("AnyValue")).ProtoReflect()
		if str != nil && *str != "" {
			v.Set(v.Descriptor().Fields().ByName("string_value"), protoreflect.ValueOfString(*str))
		}
		if i != nil {
			v.Set(v.Descriptor().Fields().ByName("int_value"), protoreflect.ValueOfInt64(*i))
		}
		m.Set(m.Descriptor().Fields().ByName("value"), protoreflect.ValueOfMessage(v))
		return m
	}
	setS := func(m protoreflect.Message, f, s string) {
		if s != "" {
			m.Set(m.Descriptor().Fields().ByName(protoreflect.Name(f)), protoreflect.ValueOfString(s))
		}
	}
	setB := func(m protoreflect.Message, f string, b []byte) {
		m.Set(m.Descriptor().Fields().ByName(protoreflect.Name(f)), protoreflect.ValueOfBytes(b))
	}
	traceID := func(id string) []byte {
		if len(id) == 32 {
			if b, err := hex.DecodeString(id); err == nil {
				return b
			}
		}
		h := sha256.Sum256([]byte(id))
		return h[:16]
	}
	spanID := func(seed string) []byte { h := sha256.Sum256([]byte(seed)); return h[:8] }

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var in input
		if err := json.Unmarshal(sc.Bytes(), &in); err != nil {
			panic(err)
		}
		res := dynamicpb.NewMessage(md("Resource")).ProtoReflect()
		attrs := res.Mutable(res.Descriptor().Fields().ByName("attributes")).List()
		sv := in.Service
		attrs.Append(protoreflect.ValueOfMessage(kv("service.name", &sv, nil)))
		for _, a := range in.Attrs {
			v := a[1]
			attrs.Append(protoreflect.ValueOfMessage(kv(a[0], &v, nil)))
		}
		scope := dynamicpb.NewMessage(md("InstrumentationScope")).ProtoReflect()
		setS(scope, "name", in.Scope)
		setS(scope, "version", in.Version)
		if in.Families != nil {
			sm := dynamicpb.NewMessage(md("ScopeMetrics")).ProtoReflect()
			sm.Set(sm.Descriptor().Fields().ByName("scope"), protoreflect.ValueOfMessage(scope))
			ms := sm.Mutable(sm.Descriptor().Fields().ByName("metrics")).List()
			for _, f := range in.Families {
				m := dynamicpb.NewMessage(md("Metric")).ProtoReflect()
				setS(m, "name", f.Name)
				setS(m, "description", f.Help)
				sort.SliceStable(f.Series, func(i, j int) bool {
					return strings.Join(f.Series[i].Values, "\x1f") < strings.Join(f.Series[j].Values, "\x1f")
				})
				pm := "NumberDataPoint"
				if f.Kind == 2 {
					pm = "HistogramDataPoint"
				}
				var pts []protoreflect.Message
				for _, sr := range f.Series {
					p := dynamicpb.NewMessage(md(pm)).ProtoReflect()
					fs := p.Descriptor().Fields()
					if f.Kind == 0 || f.Kind == 2 {
						if in.StartMs != 0 {
							p.Set(fs.ByName("start_time_unix_nano"), protoreflect.ValueOfUint64(uint64(in.StartMs*1000000)))
						}
					}
					if in.NowMs != 0 {
						p.Set(fs.ByName("time_unix_nano"), protoreflect.ValueOfUint64(uint64(in.NowMs*1000000)))
					}
					attrName := "attributes"
					if f.Kind < 2 {
						var v float64
						for _, o := range sr.Obs {
							switch f.Kind {
							case 0:
								v += o
							case 1:
								v = o
							}
						}
						p.Set(fs.ByName("as_double"), protoreflect.ValueOfFloat64(v))
					} else {
						counts := make([]uint64, len(f.Bounds)+1)
						var sum float64
						for _, o := range sr.Obs {
							k := len(f.Bounds)
							for i, b := range f.Bounds {
								if o <= b {
									k = i
									break
								}
							}
							counts[k]++
							sum += o
						}
						if n := uint64(len(sr.Obs)); n != 0 {
							p.Set(fs.ByName("count"), protoreflect.ValueOfUint64(n))
						}
						p.Set(fs.ByName("sum"), protoreflect.ValueOfFloat64(sum))
						bl := p.Mutable(fs.ByName("bucket_counts")).List()
						for _, c := range counts {
							bl.Append(protoreflect.ValueOfUint64(c))
						}
						el := p.Mutable(fs.ByName("explicit_bounds")).List()
						for _, b := range f.Bounds {
							el.Append(protoreflect.ValueOfFloat64(b))
						}
					}
					al := p.Mutable(fs.ByName(protoreflect.Name(attrName))).List()
					for i, l := range f.Labels {
						v := sr.Values[i]
						al.Append(protoreflect.ValueOfMessage(kv(l, &v, nil)))
					}
					pts = append(pts, p)
				}
				mk := func(name string) protoreflect.Message {
					d := dynamicpb.NewMessage(md(name)).ProtoReflect()
					l := d.Mutable(d.Descriptor().Fields().ByName("data_points")).List()
					for _, p := range pts {
						l.Append(protoreflect.ValueOfMessage(p))
					}
					return d
				}
				switch f.Kind {
				case 0:
					d := mk("Sum")
					d.Set(d.Descriptor().Fields().ByName("aggregation_temporality"), protoreflect.ValueOfEnum(2))
					d.Set(d.Descriptor().Fields().ByName("is_monotonic"), protoreflect.ValueOfBool(true))
					m.Set(m.Descriptor().Fields().ByName("sum"), protoreflect.ValueOfMessage(d))
				case 1:
					m.Set(m.Descriptor().Fields().ByName("gauge"), protoreflect.ValueOfMessage(mk("Gauge")))
				case 2:
					d := mk("Histogram")
					d.Set(d.Descriptor().Fields().ByName("aggregation_temporality"), protoreflect.ValueOfEnum(2))
					m.Set(m.Descriptor().Fields().ByName("histogram"), protoreflect.ValueOfMessage(d))
				}
				ms.Append(protoreflect.ValueOfMessage(m))
			}
			rm := dynamicpb.NewMessage(md("ResourceMetrics")).ProtoReflect()
			rm.Set(rm.Descriptor().Fields().ByName("resource"), protoreflect.ValueOfMessage(res))
			rm.Mutable(rm.Descriptor().Fields().ByName("scope_metrics")).List().Append(protoreflect.ValueOfMessage(sm))
			req := dynamicpb.NewMessage(md("ExportMetricsServiceRequest")).ProtoReflect()
			req.Mutable(req.Descriptor().Fields().ByName("resource_metrics")).List().Append(protoreflect.ValueOfMessage(rm))
			out, err := proto.MarshalOptions{Deterministic: true}.Marshal(req.Interface())
			if err != nil {
				panic(err)
			}
			fmt.Println(hex.EncodeToString(out))
			continue
		}
		ss := dynamicpb.NewMessage(md("ScopeSpans")).ProtoReflect()
		ss.Set(ss.Descriptor().Fields().ByName("scope"), protoreflect.ValueOfMessage(scope))
		spans := ss.Mutable(ss.Descriptor().Fields().ByName("spans")).List()
		for _, s := range in.Spans {
			sp := dynamicpb.NewMessage(md("Span")).ProtoReflect()
			root := s.Name == "request"
			setB(sp, "trace_id", traceID(s.ID))
			rootID := spanID(s.ID + "/request")
			if root {
				setB(sp, "span_id", rootID)
			} else {
				setB(sp, "span_id", spanID(fmt.Sprintf("%s/%s/%d/%d", s.ID, s.Name, s.StartMs, s.Micros)))
				setB(sp, "parent_span_id", rootID)
			}
			setS(sp, "name", s.Name)
			kind := protoreflect.EnumNumber(1)
			if root {
				kind = 2
			}
			sp.Set(sp.Descriptor().Fields().ByName("kind"), protoreflect.ValueOfEnum(kind))
			start := uint64(s.StartMs * 1000000)
			sp.Set(sp.Descriptor().Fields().ByName("start_time_unix_nano"), protoreflect.ValueOfUint64(start))
			sp.Set(sp.Descriptor().Fields().ByName("end_time_unix_nano"), protoreflect.ValueOfUint64(start+uint64(s.Micros*1000)))
			if root && s.Status > 0 {
				st := s.Status
				sp.Mutable(sp.Descriptor().Fields().ByName("attributes")).List().Append(protoreflect.ValueOfMessage(kv("http.response.status_code", nil, &st)))
				stm := dynamicpb.NewMessage(md("Status")).ProtoReflect()
				code := protoreflect.EnumNumber(1)
				if s.Status >= 500 {
					code = 2
				}
				stm.Set(stm.Descriptor().Fields().ByName("code"), protoreflect.ValueOfEnum(code))
				sp.Set(sp.Descriptor().Fields().ByName("status"), protoreflect.ValueOfMessage(stm))
			}
			spans.Append(protoreflect.ValueOfMessage(sp))
		}
		rs := dynamicpb.NewMessage(md("ResourceSpans")).ProtoReflect()
		rs.Set(rs.Descriptor().Fields().ByName("resource"), protoreflect.ValueOfMessage(res))
		rs.Mutable(rs.Descriptor().Fields().ByName("scope_spans")).List().Append(protoreflect.ValueOfMessage(ss))
		req := dynamicpb.NewMessage(md("ExportTraceServiceRequest")).ProtoReflect()
		req.Mutable(req.Descriptor().Fields().ByName("resource_spans")).List().Append(protoreflect.ValueOfMessage(rs))
		out, err := proto.MarshalOptions{Deterministic: true}.Marshal(req.Interface())
		if err != nil {
			panic(err)
		}
		fmt.Println(hex.EncodeToString(out))
	}
}
