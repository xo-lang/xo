package main

// The construct the tests use when they need exactly one XO1201 (a
// construct the native backends do not have yet): one statement, the
// local m, and the reason the diagnostic gives. When the native backends
// gain it, change it here to another construct still listed as planned,
// and every test that needs an XO1201 follows; a test with its own
// unsupported construct broke silently when that construct gained
// support (a function value with `var` parameters, fd27a4579: gotchas/
// removing-xo1201-breaks-unsupported-fixtures.md).
const (
	nativeUnsupportedStmt = "let m: Map[Set[Int], Int] = {}"
	nativeUnsupportedWhy  = "a Map key or Set element that holds a Map or Set (support is planned)"
)
