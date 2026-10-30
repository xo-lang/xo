module example.com/wf/aws
edition preview
require go github.com/aws/aws-sdk-go-v2 v1.47.5
// service/s3 is its own Go module: pinned on its own, or its binding takes
// the root's version (gowf-design.md, the nested module gap)
require go github.com/aws/aws-sdk-go-v2/service/s3 v1.114.6
require go github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.25
