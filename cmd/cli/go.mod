module github.com/agilira/argus/cmd/cli

go 1.26.0

// argus library from the parent module — replace for local development;
// when published the replace directive is dropped and the tagged version is used.
require github.com/agilira/argus v1.3.3

require (
	github.com/agilira/go-errors v1.1.3
	github.com/agilira/orpheus v1.9.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.63.0 // indirect
)

// Transitive dependencies resolved from argus and orpheus.
require (
	github.com/agilira/flash-flags v1.3.4 // indirect
	github.com/agilira/go-timecache v1.0.4 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/term v0.48.0 // indirect
)

replace github.com/agilira/argus => ../../
