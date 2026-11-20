//go:build xo_nohttp

package rt

// A program built without std/http (task #472) has no HTTP on its fake
// network: these stand in for the HTTP types of fakeNet's and
// memListener's fields, which testing_http.go gives otherwise.
type (
	fakeReq     struct{}
	memRouter   struct{}
	fakeHandler struct{}
	fakeFail    struct{}
)
