// Package hookcore implements the shared scan-then-passthrough flow that
// every package-manager shim binary executes. Each cmd/phalanx-*-hook
// configures a Config describing its ecosystem and calls Run.
package hookcore

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"

	"github.com/satnambhatt/phlx/internal/db"
	"github.com/satnambhatt/phlx/internal/scanner"
)

type PkgRef struct{ Name, Version string }

// Parser describes how to walk a package manager's argv. The npm / yarn /
// pip shapes are similar enough to share one walker.
type Parser struct {
	// Subcommands that introduce package arguments ("install", "add", ...).
	InstallSubcommands map[string]bool
	// Regex applied to each argument; capture 1 is the name, capture 2 the version.
	Regex *regexp.Regexp
	// Argument prefixes that mean "not a package" (e.g. ".", "/", "git+").
	// Do NOT include "-"; flag handling is separate.
	SkipPrefixes []string
	// Flags that consume the next argument as a value (e.g. pip's "-r FILE").
	FlagsWithValue map[string]bool
}

// Config is the ecosystem-specific dial for Run.
type Config struct {
	// "npm" or "pip". yarn uses "npm" because it shares the registry.
	Ecosystem    string
	BannerSuffix string // shown after "phalanx — "
	VersionSep   string // "@" for npm/yarn, "==" for pip
	RealNames    []string
	DefaultReal  string
	Resolve      func(name, version string) (string, error)
	Parser       Parser
}

func (c Config) parse(args []string) []PkgRef {
	idx := -1
	for i, a := range args {
		if c.Parser.InstallSubcommands[a] {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	var out []PkgRef
	for i := idx + 1; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if c.Parser.FlagsWithValue[a] && i+1 < len(args) {
				i++
			}
			continue
		}
		skip := false
		for _, p := range c.Parser.SkipPrefixes {
			if strings.HasPrefix(a, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		m := c.Parser.Regex.FindStringSubmatch(a)
		if m == nil {
			continue
		}
		v := m[2]
		if v == "" {
			v = "latest"
		}
		out = append(out, PkgRef{Name: m[1], Version: v})
	}
	return out
}

func (c Config) findReal() string {
	origPath := os.Getenv("PATH")
	parts := strings.Split(origPath, string(os.PathListSeparator))
	cleaned := parts[:0]
	for _, p := range parts {
		if !strings.Contains(p, ".phalanx") {
			cleaned = append(cleaned, p)
		}
	}
	os.Setenv("PATH", strings.Join(cleaned, string(os.PathListSeparator)))
	defer os.Setenv("PATH", origPath)
	for _, name := range c.RealNames {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return c.DefaultReal
}

func passThrough(real string, args []string) {
	cmd := exec.Command(real, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			if ws, ok := exit.Sys().(syscall.WaitStatus); ok {
				os.Exit(ws.ExitStatus())
			}
		}
		os.Exit(1)
	}
}

// Run executes the shim flow: parse args, scan every requested package,
// block if any are denied, otherwise exec the real package manager.
// Fail-open on internal errors.
func Run(cfg Config) {
	args := os.Args[1:]
	real := cfg.findReal()

	packages := cfg.parse(args)
	if len(packages) == 0 {
		passThrough(real, args)
		return
	}

	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  phalanx db error: "+err.Error()))
		passThrough(real, args)
		return
	}

	color.New(color.FgCyan, color.Bold).Printf("\n  phalanx — %s\n", cfg.BannerSuffix)
	fmt.Println()

	var blocked []string
	sp := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	sp.Color("cyan")

	for _, p := range packages {
		sp.Suffix = fmt.Sprintf("  scanning %s...", p.Name)
		sp.Start()
		resolved, err := cfg.Resolve(p.Name, p.Version)
		if err != nil {
			resolved = p.Version
		}
		if db.IsAllowed(p.Name, resolved, cfg.Ecosystem) {
			sp.Stop()
			color.Yellow("  ⚠  %s%s%s — bypassed via allow list", p.Name, cfg.VersionSep, resolved)
			db.RecordInstall(db.Install{Pkg: p.Name, Version: resolved, Ecosystem: cfg.Ecosystem, Action: "bypassed"})
			continue
		}
		res, err := scanner.Scan(scanner.Request{Pkg: p.Name, Version: resolved, Ecosystem: cfg.Ecosystem})
		sp.Stop()
		if err != nil {
			color.New(color.Faint).Printf("  ⚠  %s — scan failed (%s), proceeding\n", p.Name, err)
			db.RecordInstall(db.Install{Pkg: p.Name, Version: resolved, Ecosystem: cfg.Ecosystem, Action: "allowed"})
			continue
		}
		cfg.printResult(p.Name, resolved, res)
		action := "allowed"
		if !res.Allowed {
			action = "blocked"
			blocked = append(blocked, p.Name+cfg.VersionSep+resolved)
		} else if len(res.Policy.Warn) > 0 {
			action = "warned"
		}
		db.RecordInstall(db.Install{Pkg: p.Name, Version: resolved, Ecosystem: cfg.Ecosystem, Action: action})
	}

	if len(blocked) > 0 {
		color.New(color.FgRed, color.Bold).Printf("\n  Install cancelled — %d package(s) blocked by security policy.\n\n", len(blocked))
		os.Exit(1)
	}

	fmt.Println()
	passThrough(real, args)
}

func (c Config) printResult(pkg, version string, r scanner.FullResult) {
	if !r.Allowed {
		color.New(color.FgRed, color.Bold).Printf("  🚫 BLOCKED: %s%s%s\n", pkg, c.VersionSep, version)
		for _, d := range r.Policy.Deny {
			color.Red("     • %s", d)
		}
		color.New(color.Faint).Printf("\n  Run phalanx allow %s%s%s to bypass (not recommended)\n\n", pkg, c.VersionSep, version)
		return
	}
	if len(r.Policy.Warn) > 0 {
		color.Yellow("  ⚠  %s%s%s — %d advisory", pkg, c.VersionSep, version, len(r.Policy.Warn))
		for _, w := range r.Policy.Warn {
			color.New(color.Faint).Printf("     %s\n", w)
		}
	} else if !r.Cached {
		color.Green("  ✓  %s%s%s — clean", pkg, c.VersionSep, version)
	}
}
