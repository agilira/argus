module github.com/agilira/argus/cmd/cli

go 1.25.0

// argus library from the parent module — replace for local development;
// when published the replace directive is dropped and the tagged version is used.
require github.com/agilira/argus v1.3.3

require (
	github.com/agilira/go-errors v1.1.1
	github.com/agilira/orpheus v1.2.0
)

// Transitive dependencies resolved from argus and orpheus.
require (
	github.com/agilira/flash-flags v1.1.7 // indirect
	github.com/agilira/go-timecache v1.0.2 // indirect
	github.com/mattn/go-sqlite3 v1.14.44 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/term v0.42.0 // indirect
)

replace github.com/agilira/argus => ../../
