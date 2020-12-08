module github.com/wolfjiang/cloud-print-connector

go 1.15

require (
	github.com/coreos/go-systemd v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.1.2
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/oauth2 v0.0.0-20201203001011-0b49973bad19
	golang.org/x/sys v0.0.0-20200803210538-64077c9b5642
	launchpad.net/go-xdg v0.0.0-20140208094800-000000000010
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087
)

replace github.com/coreos/go-systemd => github.com/coreos/go-systemd/v22 v22.1.0
