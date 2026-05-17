// phalanx-pip-hook: invoked by the pip/pip3 shim. Same shape as npm hook.
package main

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
	"github.com/satnambhatt/phlx/internal/registry"
	"github.com/satnambhatt/phlx/internal/scanner"
)

type pkgRef struct{ name, version string }

func parseArgs(args []string) []pkgRef {
	idx := -1
	for i, a := range args {
		if a == "install" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	re := regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:[><=!~]+(.+))?$`)
	var out []pkgRef
	i := idx + 1
	for i < len(args) {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if (a == "-r" || a == "--requirement") && i+1 < len(args) {
				i += 2
				continue
			}
			i++
			continue
		}
		if strings.HasPrefix(a, ".") || strings.HasPrefix(a, "/") || strings.HasPrefix(a, "git+") {
			i++
			continue
		}
		m := re.FindStringSubmatch(a)
		if m != nil {
			v := m[2]
			if v == "" {
				v = "latest"
			}
			out = append(out, pkgRef{m[1], v})
		}
		i++
	}
	return out
}

func findRealPip() string {
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
	for _, c := range []string{"pip3", "pip"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return "pip3"
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

func main() {
	args := os.Args[1:]
	realPip := findRealPip()

	packages := parseArgs(args)
	if len(packages) == 0 {
		passThrough(realPip, args)
		return
	}

	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  phalanx db error: "+err.Error()))
		passThrough(realPip, args)
		return
	}

	color.New(color.FgCyan, color.Bold).Println("\n  phalanx — scanning pip packages")
	fmt.Println()

	var blocked []string
	sp := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	sp.Color("cyan")

	for _, p := range packages {
		sp.Suffix = fmt.Sprintf("  scanning %s...", p.name)
		sp.Start()
		resolved, err := registry.ResolvePypiVersion(p.name, p.version)
		if err != nil {
			resolved = p.version
		}
		if db.IsAllowed(p.name, resolved, "pip") {
			sp.Stop()
			color.Yellow("  ⚠  %s==%s — bypassed via allow list", p.name, resolved)
			db.RecordInstall(db.Install{Pkg: p.name, Version: resolved, Ecosystem: "pip", Action: "bypassed"})
			continue
		}
		res, err := scanner.Scan(scanner.Request{Pkg: p.name, Version: resolved, Ecosystem: "pip"})
		sp.Stop()
		if err != nil {
			color.New(color.Faint).Printf("  ⚠  %s — scan failed (%s), proceeding\n", p.name, err)
			db.RecordInstall(db.Install{Pkg: p.name, Version: resolved, Ecosystem: "pip", Action: "allowed"})
			continue
		}
		printPipResult(p.name, resolved, res)
		action := "allowed"
		if !res.Allowed {
			action = "blocked"
			blocked = append(blocked, p.name+"=="+resolved)
		} else if len(res.Policy.Warn) > 0 {
			action = "warned"
		}
		db.RecordInstall(db.Install{Pkg: p.name, Version: resolved, Ecosystem: "pip", Action: action})
	}

	if len(blocked) > 0 {
		color.New(color.FgRed, color.Bold).Printf("\n  Install cancelled — %d package(s) blocked.\n\n", len(blocked))
		os.Exit(1)
	}

	fmt.Println()
	passThrough(realPip, args)
}

func printPipResult(pkg, version string, r scanner.FullResult) {
	if !r.Allowed {
		color.New(color.FgRed, color.Bold).Printf("  🚫 BLOCKED: %s==%s\n", pkg, version)
		for _, d := range r.Policy.Deny {
			color.Red("     • %s", d)
		}
		color.New(color.Faint).Printf("\n  Run phalanx allow %s==%s to bypass\n\n", pkg, version)
		return
	}
	if len(r.Policy.Warn) > 0 {
		color.Yellow("  ⚠  %s==%s — %d advisory", pkg, version, len(r.Policy.Warn))
		for _, w := range r.Policy.Warn {
			color.New(color.Faint).Printf("     %s\n", w)
		}
	} else if !r.Cached {
		color.Green("  ✓  %s==%s — clean", pkg, version)
	}
}
