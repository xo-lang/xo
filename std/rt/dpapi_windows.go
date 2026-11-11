//go:build windows

package rt

import (
	"syscall"
	"unsafe"
)

// Windows' Data Protection API and known folders (decision 0584 stage E),
// the Go backend's twins of std/native/llrt/dpapi.xo and windows.xo's
// local_app_data, through syscall's lazy DLLs (no cgo).

const dpapiAvailable = true

var (
	modCrypt32               = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData     = modCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData   = modCrypt32.NewProc("CryptUnprotectData")
	modShell32               = syscall.NewLazyDLL("shell32.dll")
	procSHGetKnownFolderPath = modShell32.NewProc("SHGetKnownFolderPath")
	modOle32                 = syscall.NewLazyDLL("ole32.dll")
	procCoTaskMemFree        = modOle32.NewProc("CoTaskMemFree")
)

// dataBlob is DATA_BLOB.
type dataBlob struct {
	n uint32
	p *byte
}

func blobOf(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{n: uint32(len(b)), p: &b[0]}
}

// dpapiCrypt is CryptProtectData (protect) or CryptUnprotectData with
// CRYPTPROTECT_UI_FORBIDDEN: the output, or the Win32 error code.
func dpapiCrypt(protect bool, data, entropy []byte) ([]byte, int64) {
	in := blobOf(data)
	var ent *dataBlob
	if len(entropy) > 0 {
		ent = blobOf(entropy)
	}
	var out dataBlob
	proc := procCryptUnprotectData
	if protect {
		proc = procCryptProtectData
	}
	r, _, e := proc.Call(uintptr(unsafe.Pointer(in)), 0, uintptr(unsafe.Pointer(ent)), 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		code := int64(-1)
		if en, ok := e.(syscall.Errno); ok && en != 0 {
			code = int64(en)
		}
		return nil, code
	}
	if out.p == nil {
		return nil, 0
	}
	b := append([]byte(nil), unsafe.Slice(out.p, out.n)...)
	syscall.LocalFree(syscall.Handle(unsafe.Pointer(out.p)))
	return b, 0
}

func dpapiMessage(code int64) string {
	if code == -1 {
		return "DPAPI is not available"
	}
	return syscall.Errno(code).Error()
}

// folderIDLocalAppData is FOLDERID_LocalAppData.
var folderIDLocalAppData = syscall.GUID{Data1: 0xF1B32785, Data2: 0x6FBA, Data3: 0x4FCF, Data4: [8]byte{0x9D, 0x55, 0x7B, 0x8E, 0x7F, 0x15, 0x70, 0x91}}

// localAppData is SHGetKnownFolderPath(FOLDERID_LocalAppData), as
// golang.org/x/sys/windows KnownFolderPath; "" when it fails.
func localAppData() string {
	var p *uint16
	r, _, _ := procSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&folderIDLocalAppData)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if p == nil {
		return ""
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	if r != 0 {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), 2*n)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}
