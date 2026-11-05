# std/yaml changes

## 2026-11-05: port: std/yaml and std/yamlcodec, yaml.v3's typed decode and encode

- Kind: port
- Commit: this commit
- Found by: roadmap (Xo Lead), decision 0512 (Accepted by the owner 2026-11-05)
- Go reference: gopkg.in/yaml.v3 v3.0.1 (reader, scanner, parser, node composer, resolver, decoder, emitter, encoder, sorter)
- Test: testdata/corpus/yaml (442 documents: 91 typed cases and the yaml-test-suite's documents, with yaml.v3's value or error text; alias expansion documents; Marshal output at indent 4 and 2; omitempty and flow), testdata/corpus/yamlfuzz (seed 1, 2,000 seeded random documents against yaml.v3), `TestYamlFuzz` (fresh seeds; 8,000 documents of seeds 1 to 3 agree), golden yaml_derive, bench/perf/yamlderive (twincheck)

New `derive Yaml` (decision 0315's rule; `as "name"` is the shared wire name, `yaml "name,omitempty,flow"` a YAML only one) and the module `std/yaml`: `yaml.decode[T: Yaml](text) -> T ! YamlErr` and `yaml.encode_value[T: Yaml](v, indent = 4) -> Str ! YamlErr`. Decoding is type directed as yaml.v3's: a scalar is read as its field's type (`1` into a Str is `1`, `yes` is true only into a Bool, a float into an Int truncates), a null leaves a field alone, only the first document is read, every type error is reported together in yaml.v3's text, merge keys and aliases work, and yaml.v3's alias expansion and nesting limits apply. Encoding writes `yaml.Marshal`'s bytes. Supported fields: Str, Int, Float, Bool, Option, List, Map with Str keys, and nested structs that derive Yaml; not yet `inline`, Time, Duration, Bytes, `Node`, several documents, comments (decision 0512's later stages).

One pure source, `std/native/yamlcodec.xo` (about 5,000 lines), serves every backend (std/go_compiled.txt); `std/yamlcodec` holds the declarations of the helpers the generated code calls and is not for programs. bench/perf/yamlderive on linux/arm64 against yaml.v3 (Xo/Go wall, `--ab`): decode 0.62 (0.607 to 0.639), encode 0.604 (0.595 to 0.618) (bench/results/yaml-2026-11-05.md).

License: the port is of gopkg.in/yaml.v3, which is MIT licensed, and of libyaml (Apache-2.0), as yaml.v3's NOTICE says; the notice text is in the header of std/native/yamlcodec.xo:

```
std/yamlcodec on every backend (decision 0512): a port of gopkg.in/yaml.v3
v3.0.1 (MIT): reader, scanner, parser, node composer, resolver, the type
directed reads that derive Yaml's decoder calls, and the emitter its encoder
calls.
Copyright (c) 2011-2019 Canonical Ltd
Copyright (c) 2006-2010 Kirill Simonov
Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:
The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

The parts translated from libyaml are also covered by the Apache License,
Version 2.0 (yaml.v3's NOTICE and LICENSE).```
