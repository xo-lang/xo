// Package keep imports what the twin uses, so that `go mod vendor` keeps
// it in vendor/.
package keep

import (
	_ "google.golang.org/protobuf/proto"
	_ "google.golang.org/protobuf/types/known/structpb"
	_ "google.golang.org/protobuf/types/known/timestamppb"
)
