// phalanx-yarn-hook: invoked by the yarn shim. Configures hookcore for
// yarn (npm registry, yarn-shaped subcommands) and hands off.
package main

import (
	"regexp"

	"github.com/satnambhatt/phlx/internal/hookcore"
	"github.com/satnambhatt/phlx/internal/registry"
)

func main() {
	hookcore.Run(hookcore.Config{
		Ecosystem:    "npm",
		BannerSuffix: "scanning yarn packages",
		VersionSep:   "@",
		RealNames:    []string{"yarn"},
		DefaultReal:  "/usr/local/bin/yarn",
		Resolve:      registry.ResolveNpmVersion,
		Parser: hookcore.Parser{
			InstallSubcommands: map[string]bool{
				"add": true, "upgrade": true, "up": true,
			},
			Regex:        regexp.MustCompile(`^(@?[^@]+)(?:@(.+))?$`),
			SkipPrefixes: []string{".", "/", "file:", "git", "link:", "workspace:"},
		},
	})
}
