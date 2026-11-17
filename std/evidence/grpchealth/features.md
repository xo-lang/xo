# std/grpchealth feature audit (decision 0691)

Every feature spec/std.md section 89 and decision 0546 item 5 promise for std/grpchealth, each with the test that
holds it. The module ports grpc-go v1.79.2's `health` package; TestGrpcHealthInterop crosses Xo's server and client
with grpc-go's in all four pairings. Written 2026-11-17 (Xo Lang 3, task #260).

- [x] serving status enum with wire numbers and grpc-go's names, Other for an unnamed number | 0546 item 5 (spec/std.md 89) | std/grpchealth_test.xo
- [x] new: the server as a whole ("") SERVING from the start | 0546 item 5 | std/grpchealth_test.xo
- [x] set_serving_status per service, shared by every copy of a Health | 0546 item 5 | std/grpchealth_test.xo
- [x] shutdown and resume (every service NOT_SERVING, later sets ignored; then SERVING) | 0546 item 5 | std/grpchealth_test.xo
- [x] Check: a service's status, NotFound "unknown service" for one never set | 0546 item 5 | internal/stdtest/grpcinterop_test.go:TestGrpcHealthInterop
- [x] List: every status, ResourceExhausted past 100 services (grpc-go's maxAllowedServices) | 0546 item 5 | std/grpchealth_test.xo
- [x] Watch: the status at once (SERVICE_UNKNOWN for one never set), then each change, a repeat sent once, only the newest kept between sends | 0546 item 5 | internal/stdtest/grpcinterop_test.go:TestGrpcHealthInterop
- [x] register: Check, List and Watch served on a grpc.Server | 0546 item 5 | internal/llvm/testdata/golden/grpchealth_calls
- [x] client check, list and watch (watch ends OK when the function returns false) | 0546 item 5 | internal/llvm/testdata/golden/grpchealth_calls
- [x] request and response messages byte for byte with grpc-go's (unknown and repeated fields as protobuf reads them) | 0546 item 5 | std/grpchealth_test.xo

Not in the list: the client side health checking of grpc-go's service config (`healthCheckConfig`), which spec/std.md
89 names as not yet. It is a feature of std/grpc's service config and load balancing (map.txt's
grpc.ClientConn.GetMethodConfig and resolver rows, std/grpc's gaps), not of this module's API.
