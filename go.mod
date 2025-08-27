module github.com/O-Keh-Hunter/kcp2k-go

go 1.24.6

require github.com/xtaci/kcp-go/v5 v5.6.24

require (
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/klauspost/reedsolomon v1.12.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/xtaci/kcp-go/v5 => github.com/O-Keh-Hunter/kcp-go/v5 v5.6.24-extend-api
