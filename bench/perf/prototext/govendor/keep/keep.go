// Package keep imports what the twin uses, so that `go mod vendor` keeps
// it in vendor/ (encoding/prototext; pb holds the messages).
package keep

import (
	_ "google.golang.org/protobuf/encoding/prototext"
	_ "google.golang.org/protobuf/proto"
)
