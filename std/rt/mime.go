package rt

import (
	"mime"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// std/mime (std.md 23, decision 0282): Go's mime and net/http's
// DetectContentType. The extension table is Go's builtin one only (no
// system files), as on the native backends; a parameter that decodes to
// bytes that are not UTF-8 is dropped, as there.

func (e Mime_MimeErr) M_display(*Task) string { return e.F_msg }

// mimeBuiltin is mime's builtinTypesLower (type.go of go1.26).
var mimeBuiltin = map[string]string{
	".ai":    "application/postscript",
	".apk":   "application/vnd.android.package-archive",
	".apng":  "image/apng",
	".avif":  "image/avif",
	".bin":   "application/octet-stream",
	".bmp":   "image/bmp",
	".com":   "application/octet-stream",
	".css":   "text/css; charset=utf-8",
	".csv":   "text/csv; charset=utf-8",
	".doc":   "application/msword",
	".docx":  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".ehtml": "text/html; charset=utf-8",
	".eml":   "message/rfc822",
	".eps":   "application/postscript",
	".exe":   "application/octet-stream",
	".flac":  "audio/flac",
	".gif":   "image/gif",
	".gz":    "application/gzip",
	".htm":   "text/html; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".ico":   "image/vnd.microsoft.icon",
	".ics":   "text/calendar; charset=utf-8",
	".jfif":  "image/jpeg",
	".jpeg":  "image/jpeg",
	".jpg":   "image/jpeg",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".m4a":   "audio/mp4",
	".mjs":   "text/javascript; charset=utf-8",
	".mp3":   "audio/mpeg",
	".mp4":   "video/mp4",
	".oga":   "audio/ogg",
	".ogg":   "audio/ogg",
	".ogv":   "video/ogg",
	".opus":  "audio/ogg",
	".pdf":   "application/pdf",
	".pjp":   "image/jpeg",
	".pjpeg": "image/jpeg",
	".png":   "image/png",
	".ppt":   "application/vnd.ms-powerpoint",
	".pptx":  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".ps":    "application/postscript",
	".rdf":   "application/rdf+xml",
	".rtf":   "application/rtf",
	".shtml": "text/html; charset=utf-8",
	".svg":   "image/svg+xml",
	".text":  "text/plain; charset=utf-8",
	".tif":   "image/tiff",
	".tiff":  "image/tiff",
	".txt":   "text/plain; charset=utf-8",
	".vtt":   "text/vtt; charset=utf-8",
	".wasm":  "application/wasm",
	".wav":   "audio/wav",
	".webm":  "audio/webm",
	".webp":  "image/webp",
	".xbl":   "text/xml; charset=utf-8",
	".xbm":   "image/x-xbitmap",
	".xht":   "application/xhtml+xml",
	".xhtml": "application/xhtml+xml",
	".xls":   "application/vnd.ms-excel",
	".xlsx":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xml":   "text/xml; charset=utf-8",
	".xsl":   "text/xml; charset=utf-8",
	".zip":   "application/zip",
}

func Mime_parse_media_type(tk *Task, v string) Result[Mime_MediaType, Mime_MimeErr] {
	kind, params, err := mime.ParseMediaType(v)
	if err != nil {
		return Err[Mime_MediaType](Mime_MimeErr{F_msg: err.Error()})
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var m Map[string, string]
	for _, k := range keys {
		if utf8.ValidString(params[k]) {
			Map_put(tk, &m, k, params[k])
		}
	}
	return Ok[Mime_MediaType, Mime_MimeErr](Mime_MediaType{F_kind: kind, F_params: m})
}

func Mime_format_media_type(_ *Task, kind string, params Map[string, string]) string {
	m := map[string]string{}
	for k, v := range Map_All(params) {
		m[k] = v
	}
	return mime.FormatMediaType(kind, m)
}

func Mime_type_by_extension(_ *Task, ext string) Option[string] {
	if v, ok := mimeBuiltin[ext]; ok {
		return Some(v)
	}
	if v, ok := mimeBuiltin[strings.ToLower(ext)]; ok {
		return Some(v)
	}
	return None[string]()
}

func Mime_extensions_by_type(_ *Task, kind string) Result[List[string], Mime_MimeErr] {
	t, _, err := mime.ParseMediaType(kind)
	if err != nil {
		return Err[List[string]](Mime_MimeErr{F_msg: err.Error()})
	}
	var out []string
	for ext, v := range mimeBuiltin {
		if vt, _, err := mime.ParseMediaType(v); err == nil && vt == t {
			out = append(out, ext)
		}
	}
	slices.Sort(out)
	return Ok[List[string], Mime_MimeErr](ListFromSlice(out))
}

func Mime_detect_content_type(_ *Task, data Bytes) string {
	return mimeDetect([]byte(data))
}
