# `use go` binding coverage before and after go.Any (decision 0412), 2026-10-29

The Windows tester's survey set (their knowledgebase/findings/usego-coverage-2026-10-29.md: 47 Go std
packages and 40 third-party modules) bound with `xo-gobind --json` into one project, before with the binder
at 2817e6f78 (bugs 18 to 20 in, binding format 8) and after with decision 0412's (format 9). Both runs use
the same module versions: the 14 pins of the tester's programs, the rest the versions the first run
resolved (the second run reads them from the binding workspace). Percent = functions + methods bound /
exported. Go 1.26.1 (the tester used 1.27.0, so totals differ slightly from theirs), darwin/arm64. Method: `go run ./cmd/xo-gobind --dir PROJ --json PKG...` from each tree over the 87 paths, summed per set.

Summary: std 78.8% to 86.3% (1656 to 1813 of 2102), third party 76.9% to 93.2% (9726 to 11789 of 12648). Every "interface{} or any" skip is gone (std 157, third party 2059); generics are now the largest remaining reason (decision 0415). The 12 new "name collision after snake_case" skips are pairs that newly bind and map to one Xo name. The before column is below the tester's std 80.5% because bugs 18 and 19 (2817e6f78) now skip what Go writes into and unexported types.

## std: 47 packages: before 1656/2102 = 78.8%, after 1813/2102 = 86.3%

| package | before | after |
|---|---|---|
| database/sql | 52/89 | 85/89 |
| log/slog | 74/108 | 99/108 |
| fmt | 14/37 | 36/37 |
| log | 16/34 | 34/34 |
| sync | 22/36 | 33/36 |
| encoding/xml | 29/37 | 36/37 |
| encoding/binary | 19/25 | 25/25 |
| html/template | 23/30 | 29/30 |
| text/template | 26/33 | 32/33 |
| encoding/json | 26/35 | 31/35 |
| container/heap | 5/10 | 10/10 |
| sync/atomic | 32/86 | 36/86 |
| sort | 33/36 | 36/36 |
| os/exec | 23/25 | 25/25 |
| os | 119/131 | 121/131 |
| context | 3/16 | 4/16 |
| flag | 37/74 | 38/74 |
| slices | 0/40 | 0/40 |
| maps | 0/10 | 0/10 |
| errors | 1/6 | 1/6 |
| time | 18/37 | 18/37 |
| crypto/aes | 1/2 | 1/2 |
| mime | 7/8 | 7/8 |
| crypto/tls | 57/65 | 57/65 |
| path/filepath | 19/21 | 19/21 |
| encoding/hex | 10/13 | 10/13 |
| encoding/base64 | 11/14 | 11/14 |
| archive/zip | 32/34 | 32/34 |
| strconv | 34/36 | 34/36 |
| net/url | 38/40 | 38/40 |
| net/http | 146/153 | 146/153 |
| math/rand/v2 | 52/54 | 52/54 |
| io | 49/65 | 49/65 |
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

skip reasons before (446):
- 157 (35%) interface{} or any
- 76 (17%) pointer to a non handle type
- 60 (13%) generic function or type
- 50 (11%) output buffer ([]byte Go writes into)
- 46 (10%) method of a named basic type (the type maps to its basic type)
- 16 (4%) error outside the final result
- 15 (3%) function result or unmappable callback
- 12 (3%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 10 (2%) context.Context outside the first parameter
- 4 (1%) channel

skip reasons after (289):
- 76 (26%) pointer to a non handle type
- 60 (21%) generic function or type
- 50 (17%) output buffer ([]byte Go writes into)
- 46 (16%) method of a named basic type (the type maps to its basic type)
- 16 (6%) error outside the final result
- 14 (5%) function result or unmappable callback
- 12 (4%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 11 (4%) context.Context outside the first parameter
- 4 (1%) channel

## tp: 40 packages: before 9726/12648 = 76.9%, after 11789/12648 = 93.2%

| package | before | after |
|---|---|---|
| github.com/redis/go-redis/v9 | 7682/9331 | 9117/9331 |
| github.com/stretchr/testify/assert | 7/316 | 284/316 |
| github.com/sirupsen/logrus | 54/248 | 240/248 |
| go.uber.org/zap | 110/186 | 146/186 |
| github.com/jackc/pgx/v5 | 70/119 | 92/119 |
| github.com/rs/zerolog | 218/253 | 234/253 |
| github.com/golang-jwt/jwt/v5 | 111/130 | 124/130 |
| github.com/BurntSushi/toml | 14/25 | 25/25 |
| github.com/spf13/cobra | 139/161 | 149/161 |
| github.com/xuri/excelize/v2 | 196/206 | 204/206 |
| gopkg.in/yaml.v3 | 12/19 | 19/19 |
| github.com/gorilla/websocket | 29/43 | 35/43 |
| github.com/google/go-cmp/cmp | 42/49 | 48/49 |
| github.com/shopspring/decimal | 104/110 | 110/110 |
| github.com/samber/lo | 16/413 | 20/413 |
| github.com/tidwall/sjson | 6/10 | 10/10 |
| github.com/pelletier/go-toml/v2 | 25/32 | 29/32 |
| github.com/expr-lang/expr | 21/25 | 24/25 |
| modernc.org/sqlite | 23/31 | 25/31 |
| golang.org/x/text/language | 57/65 | 59/65 |
| github.com/pkg/errors | 2/14 | 3/14 |
| github.com/robfig/cron/v3 | 29/36 | 30/36 |
| github.com/google/uuid | 49/59 | 50/59 |
| github.com/tidwall/gjson | 34/36 | 35/36 |
| github.com/Masterminds/semver/v3 | 35/37 | 36/37 |
| github.com/hashicorp/golang-lru/v2 | 0/28 | 0/28 |
| golang.org/x/crypto/bcrypt | 3/6 | 3/6 |
| github.com/fsnotify/fsnotify | 8/12 | 8/12 |
| github.com/go-chi/chi/v5 | 68/75 | 68/75 |
| github.com/microcosm-cc/bluemonday | 36/40 | 36/40 |
| github.com/klauspost/compress/zstd | 50/55 | 50/55 |
| github.com/PuerkitoBio/goquery | 354/355 | 354/355 |
| github.com/cespare/xxhash/v2 | 14/14 | 14/14 |
| github.com/dustin/go-humanize | 27/27 | 27/27 |
| github.com/gofrs/flock | 17/17 | 17/17 |
| github.com/mailru/easyjson | 16/16 | 16/16 |
| github.com/yuin/goldmark | 15/15 | 15/15 |
| golang.org/x/crypto/argon2 | 2/2 | 2/2 |
| golang.org/x/text/cases | 9/10 | 9/10 |
| golang.org/x/time/rate | 22/22 | 22/22 |

skip reasons before (2922):
- 2059 (70%) interface{} or any
- 424 (15%) generic function or type
- 200 (7%) error outside the final result
- 122 (4%) function result or unmappable callback
- 30 (1%) method of a named basic type (the type maps to its basic type)
- 30 (1%) pointer to a non handle type
- 13 (0%) unexported type
- 11 (0%) channel
- 9 (0%) context.Context outside the first parameter
- 9 (0%) unnamed interface or struct
- 6 (0%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 3 (0%) output buffer ([]byte Go writes into)
- 3 (0%) type from another package beyond the binding depth
- 3 (0%) pointer receiver on a value type

skip reasons after (859):
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

