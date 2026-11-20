module github.com/cenkalti/rain/v2

go 1.26.0

replace github.com/nictuku/nettools => ../nettoolscompat

require (
	github.com/cenkalti/backoff/v7 v7.0.1
	github.com/cenkalti/log v1.0.0
	github.com/creachadair/jrpc2 v1.3.6
	github.com/gofrs/uuid v4.4.0+incompatible
	github.com/google/btree v1.1.3
	github.com/juju/ratelimit v1.0.2
	github.com/nictuku/dht v0.0.0-20201226073453-fd1c1dd3d66a
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/zeebo/bencode v1.0.0
	go.etcd.io/bbolt v1.5.0
	golang.org/x/sync v0.24.0
	golang.org/x/sys v0.49.0
)

require (
	github.com/creachadair/mds v0.31.0 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackpal/bencode-go v1.0.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/nictuku/nettools v0.0.0-20150117095333-8867a2107ad3 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)
