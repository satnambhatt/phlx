// Package hooks installs/removes the npm + pip shim scripts and PATH entry.
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

func Install() error {
	color.New(color.FgCyan, color.Bold).Println("\n  Installing phalanx hooks...")
	fmt.Println()

	exeDir, _ := os.Executable()
	exeDir = filepath.Dir(exeDir)

	npmHook := filepath.Join(exeDir, "phalanx-npm-hook")
	pipHook := filepath.Join(exeDir, "phalanx-pip-hook")

	// npm
	if real := findReal("npm"); real != "" {
		shim, err := writeShim("npm", npmHook)
		if err != nil {
			return err
		}
		fmt.Printf("  %s npm shim → %s\n", color.GreenString("✓"), color.New(color.Faint).Sprint(shim))
		fmt.Printf("    %s\n", color.New(color.Faint).Sprintf("real npm: %s", real))
	} else {
		fmt.Printf("  %s npm not found — skipping\n", color.YellowString("⚠"))
	}

	// pip / pip3
	pipReal := findReal("pip")
	pip3Real := findReal("pip3")
	if pipReal != "" || pip3Real != "" {
		for _, cmd := range []string{"pip", "pip3"} {
			shim, err := writeShim(cmd, pipHook)
			if err != nil {
				return err
			}
			fmt.Printf("  %s %s shim → %s\n", color.GreenString("✓"), cmd, color.New(color.Faint).Sprint(shim))
		}
		realPick := pipReal
		if realPick == "" {
			realPick = pip3Real
		}
		fmt.Printf("    %s\n", color.New(color.Faint).Sprintf("real pip: %s", realPick))
	} else {
		fmt.Printf("  %s pip/pip3 not found — skipping\n", color.YellowString("⚠"))
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

func Remove() error {
	color.New(color.FgCyan, color.Bold).Println("\n  Removing phalanx hooks...")
	pb := phalanxBin()
	for _, c := range []string{"npm", "pip", "pip3"} {
		p := filepath.Join(pb, c)
		if _, err := os.Stat(p); err == nil {
			os.Remove(p)
			fmt.Printf("  %s removed %s shim\n", color.GreenString("✓"), c)
		}
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
