# Linux test image for Xo (scripts/linux-test.sh builds and runs it).
# Go matches the host's version; clang and lld build the native backends'
# output (LLVM IR, arm64 assembly, std/llrt C) and run the sanitizers.
ARG GO_VERSION=1.26.1
FROM golang:${GO_VERSION}-trixie
RUN apt-get update \
 && apt-get install -y --no-install-recommends clang lld llvm libclang-rt-dev python3 \
 && rm -rf /var/lib/apt/lists/*
ENV CGO_ENABLED=1 GOFLAGS=-buildvcs=false GOTOOLCHAIN=local
WORKDIR /src
