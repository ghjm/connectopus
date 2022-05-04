package version

// Version - will be set by ldflags when built from Makefile
var version = "dev"

// Version returns the version number supplied during build
func Version() string {
	return version
}
