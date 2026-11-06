package main

import "google.golang.org/grpc/mem"

// nopCodec sends and receives nothing: only the headers matter.
type nopCodec struct{}

func (nopCodec) Marshal(v any) (mem.BufferSlice, error)   { return nil, nil }
func (nopCodec) Unmarshal(d mem.BufferSlice, v any) error { return nil }
func (nopCodec) Name() string                             { return "proto" }
