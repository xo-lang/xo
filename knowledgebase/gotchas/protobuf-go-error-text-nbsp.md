# protobuf-go's error texts change their spaces from one build to the next

Status: Observed (2026-11-17, TestGrpcReflectInterop)

## Symptom

TestGrpcReflectInterop failed in both pairings with grpc-go's reflection
server after a commit that only added a file to the oracle module
(internal/stdtest/grpcref): the got and want texts printed the same. A
byte diff showed `proto: not found` with U+00A0 after `proto:`.

## Cause

protobuf-go (internal/detrand) writes the space after "proto:" in its error
texts as either U+0020 or U+00A0, chosen from a hash of the running binary,
so that programs do not depend on the exact text. Any change to the binary,
such as one more file in the oracle, can flip it.

## Fix

A test that compares a protobuf-go error text (one that grpc-go's reflection
server sends as an error_message, or a Go oracle prints) normalizes U+00A0 to
a space on every side that prints it. Do not compare such a text byte for
byte with a want file.
