module github.com/localitas/localitas-app-calendar

go 1.26.3

require (
	github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608
	github.com/emersion/go-webdav v0.7.0
	github.com/grandcat/zeroconf v1.0.0
	github.com/localitas/localitas-go v0.0.0-00010101000000-000000000000
	github.com/teambition/rrule-go v1.8.2
)

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/miekg/dns v1.1.27 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/localitas/localitas-go => ../localitas-go
