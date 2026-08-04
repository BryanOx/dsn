module github.com/BryanOx/dsn/tools/replaycert

go 1.25.7

require (
	github.com/BryanOx/dsn v0.0.0
	golang.org/x/crypto v0.33.0
)

require golang.org/x/sys v0.38.0 // indirect

replace github.com/BryanOx/dsn => ../..
