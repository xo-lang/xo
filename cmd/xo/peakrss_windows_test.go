package main

import "os"

// procPeakRSS: Windows reports no peak resident set through os.ProcessState
// (the memory tests skip there).
func procPeakRSS(ps *os.ProcessState) int64 { return 0 }

const hasProcPeakRSS = false
