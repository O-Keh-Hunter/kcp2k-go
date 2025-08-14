module github.com/O-Keh-Hunter/kcp2k-go

go 1.24.6

require github.com/xtaci/kcp-go/v5 v5.6.24

require (
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/klauspost/reedsolomon v1.12.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
)

replace github.com/xtaci/kcp-go/v5 => github.com/O-Keh-Hunter/kcp-go/v5 v5.6.24-extend
