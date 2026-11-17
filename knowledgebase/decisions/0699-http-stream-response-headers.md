# 0699: Response headers from a streamed body's function, and a full duplex head sent late

Status: Proposed (xo-lang-3, 2026-11-17)

## Context

A streamed response's head is fixed when `http.stream` returns: its function cannot add a header, and a full
duplex response (decision 0433, `with_full_duplex`) sends its head before the function runs. Go's handlers set
`w.Header()` until the first Write or Flush, and EnableFullDuplex does not send the head early. grpc-go's
SetHeader, SendHeader and SetSendCompressor need this (std/grpc, decision 0546; task #89).

## Decision

To be written.
