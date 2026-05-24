// phalanx-pip-hook: invoked by the pip/pip3 shim. Configures hookcore
// for PyPI and hands off.
package main

import (
	"regexp"

	"github.com/satnambhatt/phlx/internal/hookcore"
	"github.com/satnambhatt/phlx/internal/registry"
)

func main() {
	hookcore.Run(hookcore.Config{
		Ecosystem:    "pip",
		BannerSuffix: "scanning pip packages",
		VersionSep:   "==",
		RealNames:    []string{"pip3", "pip"},
		DefaultReal:  "pip3",
		Resolve:      registry.ResolvePypiVersion,
		Parser: hookcore.Parser{
			InstallSubcommands: map[string]bool{"install": true},
			Regex:              regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:[><=!~]+(.+))?$`),
			SkipPrefixes:       []string{".", "/", "git+"},
			FlagsWithValue:     map[string]bool{"-r": true, "--requirement": true},
		},
	})
}
