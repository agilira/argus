module github.com/agilira/argus/benchmarks

go 1.23.11

require (
	github.com/agilira/argus v0.0.0
	github.com/agilira/go-timecache v1.0.2
)

require (
	github.com/agilira/flash-flags v1.1.5 // indirect
	github.com/agilira/go-errors v1.1.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.32 // indirect
)

replace github.com/agilira/argus => ../

replace github.com/agilira/go-timecache => ../../go-timecache
