//go:build xo_nohttp

package rt

import (
	"fmt"
	"sync"
)

// rpServer is a replay's server; a program built without std/http has
// none, so its trace delivers no requests (task #472).
type rpServer struct {
	reqRoot *cnode
	ended   chan struct{}
	once    sync.Once
}

// fireRequest fails: a trace of a program without std/http records no
// requests.
func (p *replayer) fireRequest(k int64) {
	p.fail(fmt.Sprintf("the trace delivers request %d, but the program does not use std/http", k))
}
