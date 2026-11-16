// Package keep imports what the twin uses, so that `go mod vendor` keeps
// it in vendor/ (encoding/protojson; pb holds the messages).
package keep

import (
	_ "google.golang.org/protobuf/encoding/protojson"
	_ "google.golang.org/protobuf/proto"
)
