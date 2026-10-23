# jsontwin: JSON decode and encode of one struct, in every language

One workload definition, written in each language's own idiom: Go
(`encoding/json`), Xo (`std/json`, `derive Json`), Rust
(`serde` + `serde_json`, ../rust/json), Node (`JSON.parse`, ../node/json) and
Python (`json`, ../python/json). Usage: `<program> decode|encode`. It prints
`<workload> <checksum>`; every language prints the same checksum.

The documents are 1000 users built by a linear congruential sequence:
`{"id":N,"name":"user N","email":"user_N@example.com","score":K.5,"active":BOOL,"tags":["t0",...]}`,
compact, ASCII, keys in that order, a score that is never a whole number (so
every language prints it the same way), 0 to 4 tags. `decode` parses the 1000
documents 100 times into the struct (Go and Rust structs, an Xo `derive Json`
struct, Node and Python plain objects) and sums fields; `encode` writes the
1000 structs 100 times and sums the output lengths.
