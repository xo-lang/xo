scope 0 "github.com/xo-lang/azure/storage/azblob" "1.8.3" "https://opentelemetry.io/schemas/1.17.0"
scope 1 "Microsoft.Storage" "2.1.0" ""
scope 2 "bare" "" ""
span 101112131415161718191a1b1c1d1e01 a000000000000001 root internal "BlobClient.Download" 1700000000000000000 1700000000000010000 scope=0
  attr "az.namespace" string "Microsoft.Storage"
span 101112131415161718191a1b1c1d1e02 a000000000000002 root client "second-scope" 1700000000000001000 1700000000000009000 scope=1
span 101112131415161718191a1b1c1d1e03 a000000000000003 root internal "first-scope-again" 1700000000000002000 1700000000000008000 scope=0
span 101112131415161718191a1b1c1d1e04 a000000000000004 root internal "no-version-no-schema" 1700000000000003000 1700000000000007000 scope=2
