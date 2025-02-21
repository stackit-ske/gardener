module github.com/gardener/gardener/hack/tools/logcheck

go 1.20

// This is a separate go module to decouple the gardener codebase and production binaries from dependencies that are
// only needed to build the logcheck tool
require (
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e
	// this has to be kept in sync with the used golangci-lint version
	// use go version -m hack/tools/bin/golangci-lint to detect the dependency versions
	golang.org/x/tools v0.9.2
)

require (
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
)
