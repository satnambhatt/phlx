// phalanx-npm-hook: invoked by the npm shim. Configures hookcore for npm
// and hands off; ecosystem-neutral logic lives in internal/hookcore.
package main

import (
	"regexp"

	"github.com/satnambhatt/phlx/internal/hookcore"
	"github.com/satnambhatt/phlx/internal/registry"
)

func main() {
	hookcore.Run(hookcore.Config{
		Ecosystem:    "npm",
		BannerSuffix: "scanning before install",
		VersionSep:   "@",
		RealNames:    []string{"npm"},
		DefaultReal:  "/usr/local/bin/npm",
		Resolve:      registry.ResolveNpmVersion,
		Parser: hookcore.Parser{
			InstallSubcommands: map[string]bool{
				"install": true, "i": true, "add": true,
				"update": true, "up": true,
			},
			Regex:        regexp.MustCompile(`^(@?[^@]+)(?:@(.+))?$`),
			SkipPrefixes: []string{".", "/", "file:", "git"},
		},
	})
}
