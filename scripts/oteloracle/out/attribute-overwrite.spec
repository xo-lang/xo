scope 0 "oracle" "" ""
span 101112131415161718191a1b1c1d1e01 a000000000000001 root internal "overwrite" 1700000000000000000 1700000000000005000 scope=0
  attr "a" int64 1
  attr "b" string "x"
  set | "a" int64 2
  set | "c" bool true | "b" string "y"
  set | "a" string "now a string"
span 101112131415161718191a1b1c1d1e02 a000000000000002 root internal "duplicate-at-start" 1700000000000000000 1700000000000005000 scope=0
  attr "d" int64 1
  attr "d" int64 2
