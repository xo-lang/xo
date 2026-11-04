# --inline-str survey (decision 0370), 2026-11-03 and 2026-11-04

Owner question (via the Lead, 2026-11-03): how does `--inline-str` (a Str of
up to 11 bytes keeps its text in its two words) affect operations other than
the ones decision 0370 measured? The default does not change.

## Method

- One xo at fe7a71942 (the 2026-11-04 rerun; the first screen at 4885673bb) on the linux/arm64 offload host 1 (12 CPUs), built by xobuild.
- Every program built twice by that xo: A default, B `--inline-str`; the
  backend, front end and binary caches off.
- Before each row, one run of A and one of B; their outputs compared (the
  first N fields where twin.txt says `compare fields N`, else the whole
  output).
- `xo bench --ab --rounds=3 --warmup=1` per row (the 3-round rule, owner
  2026-10-31; `--early` needs more than 5 rounds, so the screen ran without
  it). The first screen's worse rows and 10 best were rerun at 15 rounds
  with `--early` (2026-11-03); the rerun after the fix is the 3-round
  screen only, in chunks of at most 35 minutes (one host 1 lease each).
- Rows: every `- -` and `xo/NAME go` case of every bench/perf `twin.txt`
  (servers skipped), plus bench/perf/text's and json's measure.sh
  workloads, httpparse's five workloads (server driven by its Go load tool,
  20,000 requests; the wall is the start, the load and the stop) and
  httpclient's six (the Go test server, 3,000 requests).
- Self build: xo built by the same xo each way (xo-a, xo-b); each builds
  compiler/xo with caches off; the two outputs compared byte for byte.
- Code size: ELF `.text` (linux/arm64; darwin's `__text` counterpart) of
  xo-a and xo-b, and of three programs built `--release` each way.

Script: the lead's notes (istr-survey.sh); raw `--ab` output on the host's
/work/l1istr-out.

## Result

Before the fix (4885673bb), `--inline-str` made 158 of 339 rows worse, with
jpeg_decode 5.77x, jpeg_rgba 5.14x and xo's self build 39.6x. The cause was a
bug, not the design: under `--inline-str` every BytesBuilder.put_le and
put_be made the builder an object as `"" + cur`, a new object and a copy of
the whole builder per call (quadratic; the jpeg planes, and the linkers'
relocation patches in the self build). fe7a71942 makes it `cur + ""`
(gotchas/inline-str-put-copies-builder.md); internal/arm64
TestInlineStrPutInPlace keeps an `--inline-str` row in the full tier.

After the fix (fe7a71942, 3 rounds):

- 334 rows: 171 worse, 42 better, 121 within noise; geometric mean of
  wall B/A 1.070. Every row's output identical (the http rows compare
  their byte counts: equal on all eleven).
- jpeg_decode 1.093 (1.083 to 1.100), jpeg_rgba 1.081 (was 5.77, 5.14).
- Self build (xo built each way builds compiler/xo): wall 32,018 against
  34,342 ms, B/A 1.073 (1.065 to 1.078); CPU 1.019; max RSS 0.945 (0.94 to
  0.959); outputs identical (was 39.6x wall). The host's load was about
  6.5 in all three rounds, the same on both sides.
- Best: strconv itoa 0.466 (a short decimal string needs no allocation),
  sort strs 0.852, bytebuf large1_views 0.859.
- Worst: text cut_prefix 2.68, cut_suffix 2.50, xml none 1.99, strconv atof
  1.97, csv none 1.89, text trim 1.85, trimchars trim_start 1.85, trim
  1.84, trim_end 1.73, text cut 1.65, text hex 1.63, charset none 1.58:
  the rows that slice or cut short strings, where every slice of an inline
  Str goes through the runtime (inlstr.view) instead of sharing the object.
- HTTP: httpparse h1_get 1.071, h1_chunked 1.068, h1_url 1.103, h2_get
  1.052, h2_varied 0.998 (noise); httpclient within 1% to 2.4% (len 0.745
  is noise, spread 26%). The first screen's h2 flake did not recur.

Code size (ELF .text, linux/arm64, A then B):

| binary | default | --inline-str | B/A |
|---|---|---|---|
| xo | 33,369,996 | 34,673,672 | 1.039 |
| bench/perf/http (CRUD server) | 1,729,224 | 1,839,304 | 1.064 |
| twins/hello | 140,196 | 151,572 | 1.081 |
| bench/perf/csv | 196,276 | 214,488 | 1.093 |

Against decision 0370's own 2026-10-25 measurement (wall within noise on
Str, json and http, about 1% code size): the remaining cost is real but
small outside slicing; code size is 4 to 9%, not 1%, and slicing short
strings is the open item (a slice of an inline Str in registers for a
variable start, as calls.xo already does for a constant one).

## Rows (fe7a71942, 3 rounds)

| row | wall B/A | 95% interval | verdict | max RSS B/A | output |
|---|---|---|---|---|---|
| aead_none | 1.003 | 0.953 to 1.059 | noise | 0.95 | same |
| aead_gcm_small | 0.997 | 0.978 to 1.041 | noise | 0.95 | same |
| aead_gcm_big | 1.0 | 0.996 to 1.004 | noise | 0.976 | same |
| aead_chacha_small | 1.381 | 1.348 to 1.381 | worse | 0.95 | same |
| aead_chacha_big | 1.001 | 0.998 to 1.005 | noise | 0.981 | same |
| aead_gcm_key | 1.081 | 1.071 to 1.093 | worse | 0.95 | same |
| aead_hkdf | 1.042 | 1.038 to 1.054 | worse | 0.95 | same |
| aesgcm_seal | 0.99 | 0.985 to 0.991 | better | 0.915 | same |
| aesgcm_open | 0.997 | 0.987 to 1.0 | noise | 0.915 | same |
| aesgcm_seal1 | 0.984 | 0.982 to 0.985 | better | 0.915 | same |
| aesgcm_open1 | 0.983 | 0.981 to 0.985 | better | 0.915 | same |
| aesgcm_key | 1.097 | 1.077 to 1.111 | worse | 0.915 | same |
| base32_encode | 1.223 | 1.223 to 1.226 | worse | 1.021 | same |
| base32_decode | 1.013 | 0.995 to 1.204 | noise | 1.011 | same |
| base32_none | 0.997 | 0.973 to 1.008 | noise | 1.021 | same |
| base64_encode | 1.034 | 1.024 to 1.038 | worse | 1.021 | same |
| base64_decode | 1.016 | 1.007 to 1.025 | worse | 1.013 | same |
| base64_none | 1.004 | 0.995 to 1.02 | noise | 1.021 | same |
| bcrypt_none | 1.053 | 0.948 to 1.133 | noise | 1.0 | same |
| bcrypt_hash | 1.002 | 0.996 to 1.005 | noise | 1.0 | same |
| bcrypt_compare | 1.003 | 1.0 to 1.003 | worse | 1.0 | same |
| big_none | 1.018 | 0.919 to 1.077 | noise | 1.0 | same |
| big_mul | 0.987 | 0.975 to 1.006 | noise | 1.0 | same |
| big_quorem | 0.971 | 0.958 to 1.002 | noise | 1.0 | same |
| big_text | 1.016 | 1.005 to 1.019 | worse | 1.0 | same |
| big_parse | 0.967 | 0.963 to 0.985 | better | 1.0 | same |
| big_exp | 1.003 | 0.999 to 1.008 | noise | 1.0 | same |
| bufio_none | 0.992 | 0.992 to 1.005 | noise | 1.016 | same |
| bufio_lines | 1.03 | 1.029 to 1.042 | worse | 1.016 | same |
| bufio_until | 1.042 | 1.034 to 1.049 | worse | 1.016 | same |
| bufio_exact | 0.994 | 0.994 to 0.997 | better | 1.016 | same |
| bytebuf_none | 1.025 | 1.018 to 1.155 | worse | 1.0 | same |
| bytebuf_small_concat | 1.028 | 1.028 to 1.042 | worse | 1.065 | same |
| bytebuf_small_views | 0.989 | 0.956 to 1.05 | noise | 1.065 | same |
| bytebuf_large1_concat | 0.961 | 0.894 to 1.117 | noise | 1.038 | same |
| bytebuf_large1_views | 0.859 | 0.793 to 0.965 | better | 1.085 | same |
| bytebuf_small_buf | 1.013 | 0.978 to 1.028 | noise | 1.065 | same |
| bytebuf_small_vrec | 1.003 | 0.973 to 1.032 | noise | 1.065 | same |
| bytebuf_large1_buf | 1.002 | 0.952 to 1.061 | noise | 1.085 | same |
| bytebuf_large64_buf | 0.971 | 0.912 to 1.002 | noise | 1.002 | same |
| bytebuf_large64_views | 0.945 | 0.936 to 0.981 | better | 1.002 | same |
| bytespredicate_index | 0.988 | 0.9 to 1.07 | noise | 1.0 | same |
| bytespredicate_find_where | 0.999 | 0.944 to 0.999 | better | 1.0 | same |
| bytespredicate_rfind_where | 0.95 | 0.875 to 0.987 | better | 1.0 | same |
| bytespredicate_contains_where | 0.993 | 0.975 to 1.006 | noise | 1.0 | same |
| bytespredicate_trim_where | 0.976 | 0.941 to 0.995 | better | 1.0 | same |
| bytespredicate_trim_start_where | 1.007 | 0.993 to 1.014 | noise | 1.0 | same |
| bytespredicate_trim_end_where | 1.013 | 0.985 to 1.067 | noise | 1.0 | same |
| bytespredicate_split_where | 1.003 | 0.976 to 1.012 | noise | 1.0 | same |
| bytespredicate_map_runes | 1.194 | 1.188 to 1.201 | worse | 1.0 | same |
| bytessearch_index | 1.063 | 0.998 to 1.178 | noise | 1.0 | same |
| bytessearch_find_any | 1.054 | 1.031 to 1.08 | worse | 1.0 | same |
| bytessearch_rfind_any | 1.064 | 1.03 to 1.113 | worse | 1.0 | same |
| bytessearch_contains_any | 1.066 | 1.038 to 1.089 | worse | 1.0 | same |
| bytessearch_contains_rune | 1.002 | 0.97 to 1.019 | noise | 1.0 | same |
| bytessearch_cut | 1.038 | 1.016 to 1.038 | worse | 1.0 | same |
| bytessearch_cut_prefix | 0.929 | 0.89 to 0.958 | better | 1.0 | same |
| bytessearch_cut_suffix | 1.007 | 0.969 to 1.074 | noise | 1.0 | same |
| bytessearch_split_after | 1.138 | 1.046 to 1.145 | worse | 1.0 | same |
| bytessplit_index | 1.026 | 0.952 to 1.055 | noise | 1.0 | same |
| bytessplit_split | 1.143 | 1.13 to 1.162 | worse | 1.0 | same |
| bytessplit_split_n | 1.063 | 1.012 to 1.099 | worse | 1.0 | same |
| bytessplit_fields | 1.009 | 0.999 to 1.026 | noise | 1.0 | same |
| bytessplit_trim | 1.026 | 0.988 to 1.15 | noise | 1.0 | same |
| bytessplit_affix | 1.089 | 0.999 to 1.175 | noise | 1.0 | same |
| bytestransform_index | 1.063 | 0.919 to 1.199 | noise | 1.0 | same |
| bytestransform_replace | 1.203 | 1.173 to 1.214 | worse | 1.0 | same |
| bytestransform_replace_n | 1.207 | 1.175 to 1.229 | worse | 1.0 | same |
| bytestransform_lower | 1.018 | 0.963 to 1.02 | noise | 1.0 | same |
| bytestransform_upper | 1.061 | 0.988 to 1.093 | noise | 1.0 | same |
| bytestransform_fold | 0.936 | 0.936 to 0.993 | better | 1.0 | same |
| bytestransform_repeat | 0.977 | 0.937 to 1.016 | noise | 1.0 | same |
| bytestransform_lossy | 1.13 | 1.097 to 1.136 | worse | 1.0 | same |
| bytestransform_valid | 1.057 | 0.998 to 1.075 | noise | 1.0 | same |
| cbor_none | 1.021 | 0.99 to 1.15 | noise | 1.0 | same |
| cbor_decode | 1.01 | 0.986 to 1.026 | noise | 1.0 | same |
| cbor_encode | 0.994 | 0.988 to 0.994 | better | 1.0 | same |
| cbor_canon | 1.011 | 0.988 to 1.035 | noise | 1.0 | same |
| chacha_seal | 0.998 | 0.916 to 1.025 | noise | 0.978 | same |
| chacha_open | 1.005 | 1.002 to 1.011 | worse | 0.978 | same |
| chacha_xor | 1.004 | 1.002 to 1.004 | worse | 0.977 | same |
| chacha_poly | 1.003 | 1.002 to 1.007 | worse | 0.977 | same |
| charset_none | 1.584 | 1.558 to 1.63 | worse | 1.039 | same |
| charset_utf16_encode | 1.119 | 1.107 to 1.122 | worse | 1.03 | same |
| charset_utf16_decode | 1.07 | 1.07 to 1.08 | worse | 1.03 | same |
| charset_w1252_decode | 1.029 | 0.998 to 1.042 | noise | 1.05 | same |
| charset_w1252_encode | 1.024 | 1.008 to 1.047 | worse | 1.05 | same |
| compress_none | 1.01 | 0.969 to 1.074 | noise | 0.992 | same |
| compress_gzip | 1.002 | 1.002 to 1.005 | worse | 0.993 | same |
| compress_gzip1 | 0.992 | 0.991 to 0.994 | better | 0.992 | same |
| compress_gunzip | 0.996 | 0.995 to 1.003 | noise | 0.993 | same |
| compress_deflate | 1.004 | 1.001 to 1.004 | worse | 0.993 | same |
| compress_inflate | 0.998 | 0.994 to 1.006 | noise | 0.993 | same |
| conc.xo.spawn | 0.822 | 0.693 to 1.0 | better | 0.603 | same |
| conc.xo.fanout | 0.978 | 0.946 to 0.989 | better | 1.012 | same |
| conc.xo.pingpong | 0.989 | 0.939 to 1.009 | noise | 1.0 | same |
| conc.xo.mutex | 1.063 | 0.97 to 1.165 | noise | 1.0 | same |
| conc.xo.rwmutex | 0.988 | 0.979 to 1.026 | noise | 1.0 | same |
| conc.xo.pmap | 1.002 | 0.963 to 1.042 | noise | 1.011 | same |
| cookie_cookie | 1.308 | 1.266 to 1.413 | worse | 1.0 | same |
| cookie_setcookie | 1.501 | 1.49 to 1.563 | worse | 1.0 | same |
| cookiejar_none | 0.968 | 0.968 to 0.996 | better | 1.0 | same |
| cookiejar_set | 1.073 | 1.055 to 1.077 | worse | 1.0 | same |
| cookiejar_get | 1.114 | 1.105 to 1.119 | worse | 1.0 | same |
| cookiejar_both | 1.018 | 1.01 to 1.026 | worse | 1.0 | same |
| cron_none | 1.03 | 0.977 to 1.231 | noise | 1.0 | same |
| cron_every5 | 0.984 | 0.976 to 1.016 | noise | 1.0 | same |
| cron_weekday_ny | 1.03 | 1.021 to 1.03 | worse | 0.974 | same |
| cron_monthly_last | 0.99 | 0.921 to 0.996 | better | 1.0 | same |
| csv_none | 1.888 | 1.809 to 2.082 | worse | 0.992 | same |
| csv_parse | 1.499 | 1.466 to 1.519 | worse | 0.996 | same |
| csv_format | 1.161 | 1.101 to 1.2 | worse | 0.997 | same |
| csv_reader | 1.453 | 1.434 to 1.495 | worse | 0.992 | same |
| deque_queue | 1.011 | 0.993 to 1.029 | noise | 1.0 | same |
| deque_both | 1.005 | 0.998 to 1.013 | noise | 1.0 | same |
| deque_none | 0.906 | 0.858 to 1.07 | noise | 1.0 | same |
| dns_none | 1.04 | 0.882 to 1.14 | noise | 1.0 | same |
| dns_mx | 1.041 | 1.037 to 1.044 | worse | 1.0 | same |
| ed25519_none | 1.14 | 1.14 to 1.197 | worse | 1.0 | same |
| ed25519_keygen | 0.992 | 0.991 to 0.999 | better | 1.0 | same |
| ed25519_sign | 1.003 | 1.003 to 1.004 | worse | 1.0 | same |
| ed25519_signkey | 0.98 | 0.974 to 0.994 | better | 1.0 | same |
| ed25519_verify | 0.997 | 0.993 to 1.001 | noise | 1.0 | same |
| hash_none | 1.012 | 1.012 to 1.043 | worse | 1.0 | same |
| hash_crc32 | 1.006 | 0.999 to 1.007 | noise | 1.016 | same |
| hash_crc32c | 0.995 | 0.983 to 1.001 | noise | 1.016 | same |
| hash_crc64 | 0.993 | 0.983 to 1.003 | noise | 1.0 | same |
| hash_crc64e | 1.0 | 0.971 to 1.0 | noise | 1.0 | same |
| hash_adler32 | 0.997 | 0.968 to 0.997 | better | 1.0 | same |
| hash_fnv32a | 0.967 | 0.959 to 1.014 | noise | 1.0 | same |
| hash_fnv64a | 0.965 | 0.952 to 0.978 | better | 1.0 | same |
| hash_fnv128a | 1.011 | 0.965 to 1.055 | noise | 1.0 | same |
| heap_none | 1.144 | 1.081 to 1.162 | worse | 1.0 | same |
| heap_sort | 1.116 | 1.028 to 1.165 | worse | 1.002 | same |
| heap_steady | 0.999 | 0.984 to 1.006 | noise | 1.0 | same |
| heap_fix | 1.006 | 0.995 to 1.006 | noise | 1.009 | same |
| html_none | 1.026 | 0.865 to 1.15 | noise | 1.0 | same |
| html_escape | 1.132 | 1.122 to 1.152 | worse | 1.0 | same |
| html_unescape | 1.132 | 1.112 to 1.144 | worse | 1.0 | same |
| htmltemplate_none | 0.986 | 0.942 to 1.118 | noise | 1.048 | same |
| htmltemplate_parse | 1.044 | 1.015 to 1.059 | worse | 1.117 | same |
| htmltemplate_render | 1.034 | 1.006 to 1.048 | worse | 1.117 | same |
| idna_none | 0.949 | 0.91 to 1.02 | noise | 1.0 | same |
| idna_ascii | 1.497 | 1.473 to 1.58 | worse | 1.0 | same |
| idna_mixed | 1.165 | 1.165 to 1.179 | worse | 1.0 | same |
| idna_long | 1.09 | 1.059 to 1.107 | worse | 1.0 | same |
| idna_map | 1.292 | 1.234 to 1.338 | worse | 1.0 | same |
| idna_nfcmap | 1.239 | 1.182 to 1.246 | worse | 1.0 | same |
| idna_enc | 1.171 | 1.151 to 1.214 | worse | 1.0 | same |
| idna_dec | 1.199 | 1.176 to 1.205 | worse | 1.0 | same |
| jpeg_decode | 1.093 | 1.083 to 1.1 | worse | 1.018 | same |
| jpeg_rgba | 1.081 | 1.063 to 1.088 | worse | 1.016 | same |
| jpeg_none | 0.953 | 0.927 to 1.139 | noise | 1.0 | same |
| jsonindent_none | 0.941 | 0.842 to 1.005 | noise | 1.0 | same |
| jsonindent_indent | 1.202 | 1.175 to 1.232 | worse | 1.0 | same |
| jsonindent_rawfield | 1.273 | 1.245 to 1.288 | worse | 1.0 | same |
| jsontwin.xo_decode | 0.97 | 0.968 to 0.983 | better | 1.0 | same |
| jsontwin.xo_encode | 1.355 | 1.253 to 1.355 | worse | 1.0 | same |
| llvm.xo.fib | 1.007 | 0.988 to 1.008 | noise | 1.0 | same |
| llvm.xo.sieve | 0.993 | 0.989 to 0.993 | better | 1.001 | same |
| llvm.xo.nbody | 1.001 | 0.998 to 1.006 | noise | 1.0 | same |
| llvm.xo.sort | 1.007 | 1.004 to 1.007 | worse | 1.001 | same |
| llvm.xo.interp | 1.0 | 0.99 to 1.03 | noise | 1.0 | same |
| llvm.xo.collatz | 1.001 | 0.972 to 1.001 | noise | 1.0 | same |
| llvm.xo.strings | 1.015 | 1.008 to 1.066 | worse | 0.421 | same |
| mail_none | 0.995 | 0.975 to 1.051 | noise | 1.0 | same |
| mail_addr | 1.429 | 1.382 to 1.429 | worse | 1.0 | same |
| mail_list | 1.503 | 1.428 to 1.578 | worse | 1.0 | same |
| mail_display | 1.341 | 1.304 to 1.341 | worse | 1.0 | same |
| mail_msg | 1.071 | 1.058 to 1.089 | worse | 1.0 | same |
| mail_date | 1.133 | 1.114 to 1.182 | worse | 1.0 | same |
| mapiter_none | 0.899 | 0.884 to 0.981 | better | 1.0 | same |
| mapiter_walk | 0.98 | 0.952 to 0.988 | better | 1.0 | same |
| mapiter_keys | 1.327 | 1.002 to 1.381 | worse | 1.0 | same |
| mime_none | 1.016 | 0.967 to 1.039 | noise | 1.0 | same |
| mime_parse | 1.307 | 1.281 to 1.326 | worse | 1.0 | same |
| mime_format | 1.146 | 1.145 to 1.165 | worse | 1.0 | same |
| mime_ext | 1.064 | 1.017 to 1.091 | worse | 1.0 | same |
| mime_detect | 1.013 | 0.995 to 1.022 | noise | 1.0 | same |
| netip | 1.126 | 1.105 to 1.146 | worse | 1.0 | same |
| norm_none | 1.015 | 1.001 to 1.112 | worse | 0.975 | same |
| norm_ascii | 1.023 | 1.003 to 1.524 | worse | 0.973 | same |
| norm_nfc | 1.016 | 1.003 to 1.03 | worse | 0.982 | same |
| norm_nfd | 1.049 | 1.017 to 1.069 | worse | 0.674 | same |
| norm_nfkc | 1.027 | 1.017 to 1.029 | worse | 0.982 | same |
| otp_none | 0.98 | 0.897 to 1.039 | noise | 1.0 | same |
| otp_sha1 | 1.119 | 1.113 to 1.155 | worse | 1.0 | same |
| otp_sha256 | 1.168 | 1.155 to 1.171 | worse | 1.0 | same |
| otp_sha512 | 1.063 | 1.057 to 1.116 | worse | 1.0 | same |
| path_clean | 1.101 | 1.085 to 1.189 | worse | 1.0 | same |
| path_join | 1.094 | 1.087 to 1.101 | worse | 1.0 | same |
| path_parts | 1.391 | 1.324 to 1.441 | worse | 1.0 | same |
| path_match | 1.124 | 1.056 to 1.149 | worse | 1.0 | same |
| png_encode | 0.983 | 0.974 to 1.0 | noise | 1.004 | same |
| png_decode | 0.994 | 0.987 to 1.054 | noise | 1.004 | same |
| png_none | 0.987 | 0.986 to 1.003 | noise | 1.004 | same |
| rate_none | 1.008 | 0.985 to 1.021 | noise | 1.0 | same |
| rate_allow | 0.977 | 0.97 to 1.006 | noise | 1.0 | same |
| regexp_none | 1.099 | 1.092 to 1.13 | worse | 1.024 | same |
| regexp_compile | 0.963 | 0.958 to 0.975 | better | 1.022 | same |
| regexp_match | 1.052 | 1.028 to 1.052 | worse | 1.022 | same |
| regexp_find_all | 1.024 | 1.009 to 1.041 | worse | 1.022 | same |
| regexp_replace_all | 1.02 | 1.012 to 1.024 | worse | 1.02 | same |
| regexp_captures | 1.043 | 1.011 to 1.068 | worse | 1.022 | same |
| sha3_none | 1.029 | 0.971 to 1.038 | noise | 1.014 | same |
| sha3_big | 1.002 | 0.995 to 1.012 | noise | 1.013 | same |
| sha3_small | 1.0 | 0.992 to 1.02 | noise | 1.013 | same |
| sort.xo_ints | 0.995 | 0.992 to 0.999 | better | 0.994 | same |
| sort.xo_strs | 0.852 | 0.823 to 0.873 | better | 0.504 | same |
| sort.xo_floats | 1.008 | 1.001 to 1.014 | worse | 0.994 | same |
| sort.xo_by | 0.984 | 0.976 to 1.0 | better | 0.991 | same |
| strconv_atoi | 1.17 | 0.894 to 1.512 | noise | 1.0 | same |
| strconv_itoa | 0.466 | 0.448 to 0.475 | better | 1.0 | same |
| strconv_atof | 1.97 | 1.959 to 2.001 | worse | 1.0 | same |
| strconv_ftoa | 1.033 | 1.021 to 1.075 | worse | 1.0 | same |
| strsplit_split | 1.192 | 1.053 to 1.343 | worse | 1.0 | same |
| strsplit_split_one | 1.112 | 1.109 to 1.123 | worse | 1.0 | same |
| strsplit_split_multi | 1.369 | 1.303 to 1.376 | worse | 1.0 | same |
| strsplit_split_absent | 1.26 | 1.206 to 1.35 | worse | 1.0 | same |
| strsplit_split_empty | 1.47 | 1.45 to 1.509 | worse | 1.0 | same |
| strsplit_split_after | 1.324 | 1.291 to 1.489 | worse | 1.0 | same |
| strsplit_lines | 1.018 | 0.971 to 1.113 | noise | 1.0 | same |
| strsplit_bytes_split | 1.118 | 1.014 to 1.198 | worse | 1.0 | same |
| strsplit_bytes_fields | 1.058 | 1.034 to 1.111 | worse | 1.0 | same |
| strsplit_fields | 1.144 | 1.113 to 1.154 | worse | 1.0 | same |
| strsplit_index | 1.025 | 1.024 to 1.052 | worse | 1.0 | same |
| template_none | 0.939 | 0.905 to 1.044 | noise | 1.146 | same |
| template_parse | 1.022 | 1.013 to 1.025 | worse | 1.146 | same |
| template_render | 1.037 | 1.036 to 1.043 | worse | 1.053 | same |
| time_none | 0.893 | 0.893 to 0.94 | better | 1.0 | same |
| time_format | 1.04 | 1.024 to 1.057 | worse | 1.0 | same |
| time_parse | 0.927 | 0.921 to 0.967 | better | 1.0 | same |
| time_civil | 1.022 | 0.984 to 1.083 | noise | 1.0 | same |
| time_duration | 0.974 | 0.961 to 1.012 | noise | 1.0 | same |
| time_format_layout | 1.014 | 1.014 to 1.032 | worse | 1.0 | same |
| time_parse_layout | 0.995 | 0.884 to 0.996 | better | 1.0 | same |
| toml_none | 0.998 | 0.888 to 0.998 | better | 1.0 | same |
| toml_decode | 1.128 | 1.082 to 1.129 | worse | 1.0 | same |
| toml_encode | 1.117 | 1.112 to 1.118 | worse | 1.0 | same |
| trimchars_index | 0.996 | 0.977 to 0.997 | better | 1.0 | same |
| trimchars_trim | 1.838 | 1.804 to 2.012 | worse | 1.0 | same |
| trimchars_trim_start | 1.848 | 1.812 to 1.851 | worse | 1.0 | same |
| trimchars_trim_end | 1.728 | 1.685 to 1.728 | worse | 1.0 | same |
| twins.hello.xo | 0.939 | 0.784 to 1.113 | noise | 1.0 | same |
| unicode_lower | 0.997 | 0.985 to 1.047 | noise | 1.007 | same |
| unicode_upper | 0.985 | 0.979 to 0.986 | better | 1.007 | same |
| unicode_lower_u | 1.019 | 1.005 to 1.025 | worse | 1.007 | same |
| unicode_upper_u | 1.011 | 1.01 to 1.022 | worse | 1.007 | same |
| unicode_lower_az | 0.974 | 0.945 to 1.047 | noise | 1.007 | same |
| unicode_upper_az | 1.039 | 1.0 to 1.056 | noise | 1.007 | same |
| unicode_eqlower | 1.082 | 0.964 to 1.095 | noise | 1.007 | same |
| unicode_classify | 0.995 | 0.971 to 1.005 | noise | 1.007 | same |
| unicode_runecase | 1.005 | 0.996 to 1.015 | noise | 1.007 | same |
| unicode_classes | 1.004 | 0.992 to 1.053 | noise | 1.007 | same |
| unicode_latin1 | 1.02 | 0.961 to 1.053 | noise | 1.007 | same |
| unicode_title | 0.994 | 0.994 to 0.997 | better | 1.007 | same |
| unicode_utf8len | 0.995 | 0.87 to 1.015 | noise | 1.007 | same |
| unicode_quote | 0.987 | 0.974 to 1.013 | noise | 1.007 | same |
| unicode_indexfunc | 1.036 | 1.016 to 1.062 | worse | 1.004 | same |
| unicode_trimfunc | 0.95 | 0.95 to 1.048 | noise | 1.007 | same |
| unicode_fieldsfunc | 1.157 | 1.121 to 1.25 | worse | 1.007 | same |
| unicode_mapfunc | 1.034 | 1.01 to 1.05 | worse | 1.007 | same |
| uuid_none | 0.958 | 0.925 to 1.107 | noise | 1.0 | same |
| uuid_parse | 1.075 | 1.06 to 1.097 | worse | 1.0 | same |
| uuid_string | 1.035 | 1.022 to 1.068 | worse | 1.0 | same |
| uuid_bytes | 1.07 | 1.056 to 1.079 | worse | 1.0 | same |
| walkdir_none | 0.986 | 0.986 to 0.996 | better | 1.0 | same |
| walkdir_walk | 1.04 | 1.015 to 1.04 | worse | 0.997 | same |
| walkdir_walk_info | 1.041 | 1.027 to 1.051 | worse | 0.997 | same |
| walkdir_walk_entries | 1.017 | 0.964 to 1.042 | noise | 0.997 | same |
| walkdir_walk_entries_info | 1.05 | 1.025 to 1.084 | worse | 0.984 | same |
| websocket_none | 0.95 | 0.95 to 1.092 | noise | 1.1 | same |
| websocket_small | 0.991 | 0.953 to 0.991 | better | 1.082 | same |
| websocket_big | 1.015 | 0.865 to 1.108 | noise | 1.073 | same |
| websocket_dial | 1.024 | 1.013 to 1.037 | worse | 1.053 | same |
| x509_none | 1.06 | 0.93 to 1.117 | noise | 1.0 | same |
| x509_parse | 1.163 | 1.151 to 1.164 | worse | 0.991 | same |
| x509_verify | 0.995 | 0.975 to 0.997 | better | 0.991 | same |
| x509_create | 1.023 | 1.019 to 1.025 | worse | 1.104 | same |
| x509_crl | 1.034 | 1.01 to 1.053 | worse | 1.134 | same |
| xml_none | 1.989 | 1.838 to 2.023 | worse | 1.028 | same |
| xml_tokens | 1.07 | 1.044 to 1.077 | worse | 1.001 | same |
| xml_stream | 1.032 | 1.032 to 1.049 | worse | 1.028 | same |
| xml_tree | 1.026 | 1.017 to 1.03 | worse | 1.008 | same |
| xmlcodec_none | 1.015 | 0.976 to 1.069 | noise | 1.0 | same |
| xmlcodec_encode | 1.109 | 1.089 to 1.148 | worse | 1.0 | same |
| xmlcodec_decode | 1.05 | 1.043 to 1.05 | worse | 1.0 | same |
| text_parse_int | 0.964 | 0.928 to 1.118 | noise | 0.933 | same |
| text_dur_fmt | 1.098 | 1.073 to 1.11 | worse | 0.933 | same |
| text_dur_parse | 1.193 | 1.165 to 1.211 | worse | 0.933 | same |
| text_time_fmt | 1.047 | 1.028 to 1.05 | worse | 0.933 | same |
| text_time_parse | 1.028 | 0.991 to 1.059 | noise | 0.933 | same |
| text_quote | 1.133 | 1.119 to 1.165 | worse | 0.933 | same |
| text_split | 1.098 | 1.059 to 1.143 | worse | 0.933 | same |
| text_lines | 1.062 | 1.019 to 1.088 | worse | 0.933 | same |
| text_replace | 1.374 | 1.32 to 1.374 | worse | 0.933 | same |
| text_trim | 1.852 | 1.803 to 1.886 | worse | 0.933 | same |
| text_rune_count | 1.03 | 0.97 to 1.03 | noise | 0.933 | same |
| text_cut | 1.652 | 1.578 to 1.721 | worse | 0.936 | same |
| text_cut_prefix | 2.68 | 2.553 to 2.842 | worse | 0.936 | same |
| text_cut_suffix | 2.496 | 2.346 to 2.635 | worse | 0.936 | same |
| text_utf8 | 1.018 | 0.974 to 1.075 | noise | 0.933 | same |
| text_int_fmt | 1.211 | 1.182 to 1.246 | worse | 0.933 | same |
| text_float_fmt | 1.109 | 1.077 to 1.137 | worse | 0.933 | same |
| text_float_parse | 1.127 | 1.083 to 1.128 | worse | 0.933 | same |
| text_base64 | 1.405 | 1.322 to 1.454 | worse | 0.933 | same |
| text_hex | 1.626 | 1.587 to 1.633 | worse | 0.933 | same |
| text_debug | 1.218 | 1.218 to 1.231 | worse | 0.933 | same |
| text_zone_offset | 0.994 | 0.948 to 1.049 | noise | 0.942 | same |
| text_zone_fmt | 1.046 | 1.025 to 1.087 | worse | 0.942 | same |
| text_zone_civil | 1.19 | 1.166 to 1.227 | worse | 0.942 | same |
| text_zone_load | 1.008 | 0.96 to 1.064 | noise | 0.942 | same |
| json_small | 1.071 | 0.983 to 1.121 | noise | 0.712 | same |
| json_ints | 1.022 | 1.011 to 1.077 | worse | 0.699 | same |
| json_floats | 1.005 | 0.968 to 1.039 | noise | 0.699 | same |
| json_strings | 1.083 | 1.072 to 1.091 | worse | 0.699 | same |
| json_maps | 1.156 | 1.138 to 1.159 | worse | 0.712 | same |
| json_parse | 0.982 | 0.965 to 0.982 | better | 0.755 | same |
| json_invalid | 1.0 | 0.934 to 1.107 | noise | 0.712 | same |
| json_enc_small | 1.163 | 1.14 to 1.183 | worse | 0.699 | same |
| json_enc_strings | 1.013 | 1.009 to 1.092 | worse | 0.699 | same |
| json_enc_floats | 1.102 | 1.094 to 1.151 | worse | 0.712 | same |
| json_bytes | 1.17 | 1.152 to 1.218 | worse | 1.002 | same |
| httpparse_h1_get | 1.071 | 1.047 to 1.071 | worse | 1.002 |  |
| httpparse_h1_chunked | 1.068 | 1.036 to 1.091 | worse | 0.998 |  |
| httpparse_h1_url | 1.103 | 1.097 to 1.106 | worse | 1.003 |  |
| httpparse_h2_get | 1.052 | 1.03 to 1.057 | worse | 1.0 |  |
| httpparse_h2_varied | 0.998 | 0.998 to 1.017 | noise | 0.891 |  |
| httpclient_len | 0.745 | 0.745 to 1.016 | noise | 1.0 | 792000 / 792000 |
| httpclient_chunked | 1.006 | 0.991 to 1.007 | noise | 1.0 | 12309000 / 12309000 |
| httpclient_big | 1.007 | 1.003 to 1.017 | worse | 1.0 | 196632000 / 196632000 |
| httpclient_redirect | 1.01 | 1.007 to 1.011 | worse | 1.0 | 792000 / 792000 |
| httpclient_h2len | 1.007 | 1.0 to 1.036 | worse | 1.037 | 792000 / 792000 |
| httpclient_h2big | 1.024 | 1.008 to 1.109 | worse | 1.263 | 196629000 / 196629000 |
