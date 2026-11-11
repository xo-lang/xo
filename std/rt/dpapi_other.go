//go:build !windows

package rt

// DPAPI is Windows only (decision 0584): Secrets.dpapi() is None
// elsewhere outside tests, so dpapiCrypt is never reached.

const dpapiAvailable = false

func dpapiCrypt(protect bool, data, entropy []byte) ([]byte, int64) { return nil, -1 }

func dpapiMessage(code int64) string { return "DPAPI is not available" }

// localAppData is "" off Windows (Env.local_app_data is Missing).
func localAppData() string { return "" }
