# `use go` binding coverage after decisions 0415 and 0416, 2026-10-29

The same survey set and method as usego-coverage-2026-10-29.md (the Windows tester's 47 Go std packages
and 40 third-party modules, one project, the same module versions, `go run ./cmd/xo-gobind --dir PROJ
--json PKG...` over the 87 paths, Go 1.26.1, darwin/arm64). Before: the binder at 1554420ee (decision
0412, binding format 9), the "after" column of usego-coverage-2026-10-29.md. After: the binder with
0415 stages 1 and 2 (generic functions and types), 0416 steps 1, 2 and 4 (callbacks returning errors,
implementations, `go.Error?` for errors outside a final result) and the upcasts (binding format 15).
Percent = functions + methods bound / exported; constructors of implementations and `as_<iface>()`
upcasts are not counted (they are not Go functions).

Summary: std 86.3% to 87.4% (1813 to 1838 of 2102), third party 93.2% to 97.6% (11789 to 12345 of
12648, 12650 after: go.uber.org/zap and samber/lo each export one more method under the binder's view of
generics). The reasons removed: "generic function or type" in third party 424 to 25 (std 60 to 52: Go's
slices and maps stay unbound by the owner's decision, 50 functions) and "error outside the final result"
(std 16 and third party 202, to none). Pointers to non handle types rose in third party (30 to 42) because
newly bound types expose them. The largest remaining third-party reason is "function result or unmappable
callback" (109), then those pointers (decision 0419 proposes `T?` copies).

## std: 47 packages: before 1813/2102 = 86.3%, after 1838/2102 = 87.4%

| package | before | after |
|---|---|---|
| os | 121/131 | 126/131 |
| errors | 1/6 | 5/6 |
| flag | 38/74 | 42/74 |
| database/sql | 85/89 | 88/89 |
| path/filepath | 19/21 | 21/21 |
| archive/zip | 32/34 | 34/34 |
| io | 49/65 | 51/65 |
| html/template | 29/30 | 30/30 |
| text/template | 32/33 | 33/33 |
| net/http | 146/153 | 147/153 |
| slices | 0/40 | 0/40 |
| maps | 0/10 | 0/10 |
| context | 4/16 | 4/16 |
| sync/atomic | 36/86 | 36/86 |
| fmt | 36/37 | 36/37 |
| log | 34/34 | 34/34 |
| time | 18/37 | 18/37 |
| encoding/json | 31/35 | 31/35 |
| crypto/aes | 1/2 | 1/2 |
| container/heap | 10/10 | 10/10 |
| sync | 33/36 | 33/36 |
| log/slog | 99/108 | 99/108 |
| encoding/binary | 25/25 | 25/25 |
| encoding/xml | 36/37 | 36/37 |
| mime | 7/8 | 7/8 |
| crypto/tls | 57/65 | 57/65 |
| sort | 36/36 | 36/36 |
| os/exec | 25/25 | 25/25 |
| encoding/hex | 10/13 | 10/13 |
| encoding/base64 | 11/14 | 11/14 |
| strconv | 34/36 | 34/36 |
| net/url | 38/40 | 38/40 |
| math/rand/v2 | 52/54 | 52/54 |
| net | 223/247 | 223/247 |
| bufio | 59/62 | 59/62 |
| bytes | 92/95 | 92/95 |
| strings | 77/79 | 77/79 |
| math | 67/67 | 67/67 |
| regexp | 48/48 | 48/48 |
| unicode/utf8 | 15/15 | 15/15 |
| encoding/csv | 12/12 | 12/12 |
| crypto/sha256 | 4/4 | 4/4 |
| crypto/hmac | 2/2 | 2/2 |
| crypto/rand | 3/4 | 3/4 |
| compress/gzip | 10/11 | 10/11 |
| net/mail | 10/10 | 10/10 |
| hash/crc32 | 6/6 | 6/6 |

skip reasons before (289):
- 76 (26%) pointer to a non handle type
- 60 (21%) generic function or type
- 50 (17%) output buffer ([]byte Go writes into)
- 46 (16%) method of a named basic type (the type maps to its basic type)
- 16 (6%) error outside the final result
- 14 (5%) function result or unmappable callback
- 12 (4%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 11 (4%) context.Context outside the first parameter
- 4 (1%) channel

skip reasons after (264):
- 80 (30%) pointer to a non handle type
- 52 (20%) generic function or type
- 50 (19%) output buffer ([]byte Go writes into)
- 46 (17%) method of a named basic type (the type maps to its basic type)
- 13 (5%) context.Context outside the first parameter
- 12 (5%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 7 (3%) function result or unmappable callback
- 4 (2%) channel

## tp: 40 packages: before 11789/12648 = 93.2%, after 12345/12650 = 97.6%

| package | before | after |
|---|---|---|
| github.com/samber/lo | 20/413 | 285/414 |
| github.com/redis/go-redis/v9 | 9117/9331 | 9280/9331 |
| github.com/stretchr/testify/assert | 284/316 | 316/316 |
| github.com/hashicorp/golang-lru/v2 | 0/28 | 28/28 |
| github.com/jackc/pgx/v5 | 92/119 | 106/119 |
| github.com/pkg/errors | 3/14 | 12/14 |
| github.com/rs/zerolog | 234/253 | 243/253 |
| go.uber.org/zap | 146/186 | 153/187 |
| modernc.org/sqlite | 25/31 | 31/31 |
| github.com/sirupsen/logrus | 240/248 | 245/248 |
| github.com/gorilla/websocket | 35/43 | 40/43 |
| github.com/golang-jwt/jwt/v5 | 124/130 | 128/130 |
| github.com/google/uuid | 50/59 | 52/59 |
| github.com/spf13/cobra | 149/161 | 151/161 |
| github.com/pelletier/go-toml/v2 | 29/32 | 30/32 |
| github.com/robfig/cron/v3 | 30/36 | 31/36 |
| github.com/Masterminds/semver/v3 | 36/37 | 37/37 |
| github.com/xuri/excelize/v2 | 204/206 | 205/206 |
| github.com/PuerkitoBio/goquery | 354/355 | 355/355 |
| golang.org/x/crypto/bcrypt | 3/6 | 3/6 |
| github.com/BurntSushi/toml | 25/25 | 25/25 |
| github.com/tidwall/sjson | 10/10 | 10/10 |
| gopkg.in/yaml.v3 | 19/19 | 19/19 |
| github.com/fsnotify/fsnotify | 8/12 | 8/12 |
| github.com/expr-lang/expr | 24/25 | 24/25 |
| github.com/google/go-cmp/cmp | 48/49 | 48/49 |
| golang.org/x/text/language | 59/65 | 59/65 |
| github.com/go-chi/chi/v5 | 68/75 | 68/75 |
| github.com/microcosm-cc/bluemonday | 36/40 | 36/40 |
| github.com/klauspost/compress/zstd | 50/55 | 50/55 |
| github.com/tidwall/gjson | 35/36 | 35/36 |
| github.com/shopspring/decimal | 110/110 | 110/110 |
| github.com/cespare/xxhash/v2 | 14/14 | 14/14 |
| github.com/dustin/go-humanize | 27/27 | 27/27 |
| github.com/gofrs/flock | 17/17 | 17/17 |
| github.com/mailru/easyjson | 16/16 | 16/16 |
| github.com/yuin/goldmark | 15/15 | 15/15 |
| golang.org/x/crypto/argon2 | 2/2 | 2/2 |
| golang.org/x/text/cases | 9/10 | 9/10 |
| golang.org/x/time/rate | 22/22 | 22/22 |

skip reasons before (859):
- 424 (49%) generic function or type
- 202 (24%) error outside the final result
- 104 (12%) function result or unmappable callback
- 30 (3%) method of a named basic type (the type maps to its basic type)
- 30 (3%) pointer to a non handle type
- 13 (2%) unexported type
- 12 (1%) name collision after snake_case
- 11 (1%) channel
- 9 (1%) context.Context outside the first parameter
- 9 (1%) unnamed interface or struct
- 6 (1%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 3 (0%) output buffer ([]byte Go writes into)
- 3 (0%) type from another package beyond the binding depth
- 3 (0%) pointer receiver on a value type

skip reasons after (305):
- 109 (36%) function result or unmappable callback
- 42 (14%) pointer to a non handle type
- 30 (10%) method of a named basic type (the type maps to its basic type)
- 26 (9%) channel
- 25 (8%) generic function or type
- 18 (6%) unexported type
- 12 (4%) name collision after snake_case
- 10 (3%) unnamed interface or struct
- 9 (3%) more than 6 results
- 9 (3%) context.Context outside the first parameter
- 6 (2%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 3 (1%) output buffer ([]byte Go writes into)
- 3 (1%) type from another package beyond the binding depth
- 3 (1%) pointer receiver on a value type

