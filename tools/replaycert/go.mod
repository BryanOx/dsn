module github.com/dsn/dsn/tools/replaycert

go 1.25.7

require (
	github.com/dsn/dsn v0.0.0
	golang.org/x/crypto v0.33.0
)

require golang.org/x/sys v0.38.0 // indirect

replace github.com/dsn/dsn => ../..
