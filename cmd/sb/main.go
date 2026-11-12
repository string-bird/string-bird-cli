// Command sb is the StringBird client CLI.
package main

import (
	"os"
	"runtime/debug"

	"github.com/string-bird/string-bird-cli/internal/cli"
)

// version is set by release builds: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	os.Exit(cli.Execute(buildVersion()))
}

// buildVersion falls back to the module version for go install builds.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
