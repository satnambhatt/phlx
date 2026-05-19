// Package hooks installs/removes the npm + pip + yarn shim scripts and PATH entry.
package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

const pathLine = "\n# phalanx — package security hooks\nexport PATH=\"$HOME/.phalanx/bin:$PATH\"\n"

var shellFiles = []string{".zshrc", ".zprofile", ".bashrc", ".bash_profile", ".profile"}

// Spec describes a single shim Phalanx can install.
type Spec struct {
	Name     string // shim filename under ~/.phalanx/bin/
	RealName string // command name to look up on PATH
	Binary   string // phalanx-*-hook binary the shim execs
}

// AvailableHooks is the closed set of shims `phlx hooks install` understands.
// Order matters — used for both ValidArgs in the CLI and default install order.
var AvailableHooks = []Spec{
	{Name: "npm", RealName: "npm", Binary: "phalanx-npm-hook"},
	{Name: "yarn", RealName: "yarn", Binary: "phalanx-yarn-hook"},
	{Name: "pip", RealName: "pip", Binary: "phalanx-pip-hook"},
	{Name: "pip3", RealName: "pip3", Binary: "phalanx-pip-hook"},
}

// AvailableHookNames returns the shim names as a flat slice — handy for
// cobra ValidArgs and user-facing error messages.
func AvailableHookNames() []string {
	out := make([]string, len(AvailableHooks))
	for i, h := range AvailableHooks {
		out[i] = h.Name
	}
	return out
}

func phalanxBin() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".phalanx", "bin")
}

func shimScript(hookBinary string) string {
	return fmt.Sprintf("#!/usr/bin/env bash\n# phalanx shim\nexec %q \"$@\"\n", hookBinary)
}

// findReal looks up the real npm/pip binary with the phalanx bin dir removed.
func findReal(cmd string) string {
	pb := phalanxBin()
	origPath := os.Getenv("PATH")
	parts := strings.Split(origPath, string(os.PathListSeparator))
	cleaned := parts[:0]
	for _, p := range parts {
		if !strings.Contains(p, ".phalanx") && p != pb {
			cleaned = append(cleaned, p)
		}
	}
	os.Setenv("PATH", strings.Join(cleaned, string(os.PathListSeparator)))
	defer os.Setenv("PATH", origPath)
	if p, err := exec.LookPath(cmd); err == nil {
		return p
	}
	return ""
}

func writeShim(name, hookBinary string) (string, error) {
	pb := phalanxBin()
	if err := os.MkdirAll(pb, 0o755); err != nil {
		return "", err
	}
	shim := filepath.Join(pb, name)
	if err := os.WriteFile(shim, []byte(shimScript(hookBinary)), 0o755); err != nil {
		return "", err
	}
	return shim, nil
}

// selectSpecs resolves the requested hook names against AvailableHooks.
// Empty selection means "everything". Unknown names are reported in err.
func selectSpecs(names []string) ([]Spec, error) {
	if len(names) == 0 {
		out := make([]Spec, len(AvailableHooks))
		copy(out, AvailableHooks)
		return out, nil
	}
	byName := make(map[string]Spec, len(AvailableHooks))
	for _, h := range AvailableHooks {
		byName[h.Name] = h
	}
	var out []Spec
	var unknown []string
	for _, n := range names {
		spec, ok := byName[n]
		if !ok {
			unknown = append(unknown, n)
			continue
		}
		out = append(out, spec)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown hook(s): %s — valid: %s",
			strings.Join(unknown, ", "), strings.Join(AvailableHookNames(), ", "))
	}
	return out, nil
}

// Install writes shims for the requested hooks. If selected is empty, every
// hook whose real binary is on PATH is installed. When the user explicitly
// names a hook, the shim is written even if the real binary is missing —
// the user is opting in for a manager they plan to install later.
func Install(selected ...string) error {
	color.New(color.FgCyan, color.Bold).Println("\n  Installing phalanx hooks...")
	fmt.Println()

	specs, err := selectSpecs(selected)
	if err != nil {
		return err
	}
	explicit := len(selected) > 0

	exeDir, _ := os.Executable()
	exeDir = filepath.Dir(exeDir)

	wrote := 0
	for _, spec := range specs {
		hookBinary := filepath.Join(exeDir, spec.Binary)
		real := findReal(spec.RealName)
		if real == "" && !explicit {
			fmt.Printf("  %s %s not found — skipping\n", color.YellowString("⚠"), spec.RealName)
			continue
		}
		shim, err := writeShim(spec.Name, hookBinary)
		if err != nil {
			return err
		}
		fmt.Printf("  %s %s shim → %s\n", color.GreenString("✓"), spec.Name, color.New(color.Faint).Sprint(shim))
		if real == "" {
			fmt.Printf("    %s\n", color.New(color.Faint).Sprintf("real %s: not on PATH — shim will fail open until installed", spec.RealName))
		} else {
			fmt.Printf("    %s\n", color.New(color.Faint).Sprintf("real %s: %s", spec.RealName, real))
		}
		wrote++
	}

	if wrote == 0 {
		fmt.Printf("  %s no hooks installed\n", color.YellowString("⚠"))
	}

	// Append PATH entry to first existing shell rc
	home, _ := os.UserHomeDir()
	addedTo := ""
	for _, name := range shellFiles {
		p := filepath.Join(home, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), ".phalanx/bin") {
			addedTo = p
			break
		}
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		f.WriteString(pathLine)
		f.Close()
		fmt.Printf("  %s PATH updated in %s\n", color.GreenString("✓"), color.New(color.Faint).Sprint(p))
		addedTo = p
		break
	}
	if addedTo == "" {
		fmt.Printf("  %s no shell config found. Add manually:\n", color.YellowString("⚠"))
		fmt.Println(color.New(color.Faint).Sprint(`    export PATH="$HOME/.phalanx/bin:$PATH"`))
	}

	fmt.Println()
	color.New(color.FgGreen).Println("  ✓ Hooks installed")
	if addedTo != "" {
		rcPath := tildeify(addedTo, home)
		fmt.Println(color.New(color.Faint).Sprint("\n  Reload your shell or run:"))
		fmt.Printf("    source %s\n", rcPath)
	} else {
		fmt.Println(color.New(color.Faint).Sprint("\n  Open a new terminal to pick up the new PATH."))
	}
	fmt.Println()
	return nil
}

func tildeify(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// Remove deletes the requested shims. Empty selection removes every shim
// Phalanx knows about and strips the PATH line. With an explicit selection
// the PATH line stays — there are still other shims that need it.
func Remove(selected ...string) error {
	color.New(color.FgCyan, color.Bold).Println("\n  Removing phalanx hooks...")
	specs, err := selectSpecs(selected)
	if err != nil {
		return err
	}
	pb := phalanxBin()
	for _, spec := range specs {
		p := filepath.Join(pb, spec.Name)
		if _, err := os.Stat(p); err == nil {
			os.Remove(p)
			fmt.Printf("  %s removed %s shim\n", color.GreenString("✓"), spec.Name)
		}
	}
	if len(selected) > 0 {
		fmt.Println(color.New(color.Faint).Sprint("\n  PATH line left in place — other shims may still be active."))
		fmt.Println()
		return nil
	}
	home, _ := os.UserHomeDir()
	for _, name := range shellFiles {
		p := filepath.Join(home, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if !strings.Contains(string(data), ".phalanx/bin") {
			continue
		}
		newContent := strings.Replace(string(data), pathLine, "", 1)
		os.WriteFile(p, []byte(newContent), 0o644)
		fmt.Printf("  %s cleaned PATH from %s\n", color.GreenString("✓"), p)
	}
	fmt.Println()
	return nil
}
