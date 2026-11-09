scope 0 "oracle" "" ""
span 101112131415161718191a1b1c1d1e01 a000000000000001 root internal "events" 1700000000000000000 1700000000000050000 scope=0
  event "plain" 1700000000000010000
  event "with-attrs" 1700000000000020000
    eattr "s" string "v"
    eattr "i" int64 -1
    eattr "f" float64 0.5
    eattr "b" bool false
  event "plain" 1700000000000030000
  event "same-time" 1700000000000030000
    eattr "s" string ""
