# std/gif changes

## 2026-11-09: feature: std/gif (decision 0578 step 5)

- Kind: feature
- Commit: this push
- Found by: decision 0578 (a GIF codec on the shared std/image Image)
- Go reference: image/gif reader.go and writer.go (error texts, blockReader's too much image data check through a resumable batch LZW reader, interlace, transparent index beyond the palette, local table reuse of the global one, NETSCAPE2.0 loop block)
- Test: testdata/corpus/gif: encoded animations, truncations, byte changes, hand built streams for each error text, Encode over Paletted, RGBA, NRGBA and Gray, EncodeAll over 40 animations
- Timing: bench/perf/gif twincheck on the Mac against Go's image/gif, 5 rounds: encode_all 1.03, decode_all 0.88, setup 0.92 (the decoder's LZW batch writes each span back to front through the prefix table, as Go's does; it was 1.31 with a per-byte builder). host-1 row to follow in the next survey
- Shipped in: not yet released

`decode`, `decode_all`, `decode_config`, `encode`, `encode_all`, `Gif`, `Config`, `GifErr`. std/compress gains a resumable LZW reader for it (native, not public API).
