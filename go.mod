module github.com/abhisekp/open-download-manager

go 1.26.0

toolchain go1.26.11

require (
	github.com/cenkalti/log v1.0.0
	github.com/cenkalti/rain/v2 v2.5.1
	github.com/gofrs/flock v0.13.2
	github.com/tetratelabs/wazero v1.12.0
	github.com/zalando/go-keyring v0.2.8
	github.com/zeebo/bencode v1.0.0
	go.etcd.io/bbolt v1.5.0
	golang.org/x/time v0.16.0
)

require (
	github.com/cenkalti/backoff/v7 v7.0.1 // indirect
	github.com/creachadair/jrpc2 v1.3.6 // indirect
	github.com/creachadair/mds v0.31.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackpal/bencode-go v1.0.0 // indirect
	github.com/juju/ratelimit v1.0.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/nictuku/dht v0.0.0-20201226073453-fd1c1dd3d66a // indirect
	github.com/nictuku/nettools v0.0.0-20150117095333-8867a2107ad3 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/nictuku/nettools => ./internal/nettoolscompat

replace github.com/cenkalti/rain/v2 => ./internal/raincompat
