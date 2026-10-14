; fault_shared.ll: the catcher of fault_shared.c, a frame of "generated
; code": its landing pad is under xo_x_personality, so the Xo unwinder
; enters it.
declare void @call_qsort()
declare void @caught()
declare i32 @xo_x_personality(...)

define void @catcher() personality ptr @xo_x_personality {
entry:
  invoke void @call_qsort() to label %ok unwind label %lp
ok:
  ret void
lp:
  %x = landingpad { ptr, i32 } cleanup
  call void @caught()
  unreachable
}
