module github.com/Code-Hex/vz/example

go 1.16

replace github.com/Code-Hex/vz => ../

//replace github.com/containers/gvisor-tap-vsock => ../../gvisor-tap-vsock

require (
	github.com/Code-Hex/vz v0.0.3
	github.com/containers/gvisor-tap-vsock v0.3.1-0.20220309080941-bda57eac5e52
	github.com/dustin/go-humanize v1.0.0
	github.com/miekg/dns v1.1.50 // indirect
	github.com/pkg/errors v0.9.1
	github.com/pkg/term v1.1.0
	github.com/rs/xid v1.3.0 // indirect
	github.com/sirupsen/logrus v1.9.0
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f
)
