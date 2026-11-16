// Package keep imports what the twin uses, so that `go mod vendor` keeps
// it in vendor/.
package keep

import (
	_ "github.com/bufbuild/protocompile/parser"
	_ "github.com/bufbuild/protocompile/reporter"
)
