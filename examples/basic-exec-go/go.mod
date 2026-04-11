module github.com/xgen-sandbox/examples/basic-exec-go

go 1.22

require github.com/xgen-sandbox/sdk-go v0.0.0

require (
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	nhooyr.io/websocket v1.8.11 // indirect
)

replace github.com/xgen-sandbox/sdk-go => ../../sdks/go
