// Package cli wires Cobra commands for the phalanx binary.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/satnambhatt/phlx/internal/db"
	"github.com/satnambhatt/phlx/internal/hooks"
	"github.com/satnambhatt/phlx/internal/scanner"
)

// Root builds the cobra command tree. The version string is wired in by
// the binary entrypoint so goreleaser's -X main.version flows through to
// `phlx --version` and the self-updater.
func Root(version string) *cobra.Command {
	if version == "" {
		version = "dev"
	}
	root := &cobra.Command{
		Use:           "phalanx",
		Short:         "phalanx — package security that holds the line",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(
		statusCmd(),
		scanCmd(),
		historyCmd(),
		watchCmd(),
		allowCmd(),
		hooksCmd(),
		configCmd(),
		updateCmd(version),
	)
	return root
}

func banner() {
	fmt.Println()
	color.New(color.FgCyan, color.Bold).Print("  phalanx")
	color.New(color.Faint).Println(" — holds the line")
	fmt.Println()
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show scan stats and recent activity",
		RunE: func(cmd *cobra.Command, args []string) error {
			banner()
			s, err := db.GetStats()
			if err != nil {
				return err
			}
			row := func(label string, val int, c *color.Color) {
				fmt.Printf("  %-22s %s\n", color.New(color.Faint).Sprint(label), c.Sprint(val))
			}
			row("Total scans", s.TotalScans, color.New(color.FgWhite))
			row("Allowed", s.Allowed, color.New(color.FgGreen))
			row("Warned", s.Warned, color.New(color.FgYellow))
			row("Blocked", s.Blocked, color.New(color.FgRed))
			row("Watched projects", s.Watched, color.New(color.FgBlue))
			fmt.Println()
			return nil
		},
	}
}

func scanCmd() *cobra.Command {
	var ecosystem string
	cmd := &cobra.Command{
		Use:   "scan <package>",
		Short: "Manually scan a package  e.g. phalanx scan lodash@4.17.21",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			banner()
			pkg, version := parsePkgArg(args[0])

			sp := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			sp.Suffix = fmt.Sprintf("  scanning %s@%s...", pkg, version)
			sp.Color("cyan")
			sp.Start()
			result, err := scanner.Scan(scanner.Request{Pkg: pkg, Version: version, Ecosystem: ecosystem})
			sp.Stop()
			if err != nil {
				color.Red("  Scan failed: %s", err)
				return err
			}

			printResult(pkg, version, result)
			action := actionFor(result)
			_ = db.RecordInstall(db.Install{Pkg: pkg, Version: version, Ecosystem: ecosystem, Action: action})
			fmt.Println()
			return nil
		},
	}
	cmd.Flags().StringVarP(&ecosystem, "ecosystem", "e", "npm", "npm or pip")
	return cmd
}

func actionFor(r scanner.FullResult) string {
	if !r.Allowed {
		return "blocked"
	}
	if len(r.Policy.Warn) > 0 {
		return "warned"
	}
	return "allowed"
}

func printResult(pkg, version string, r scanner.FullResult) {
	switch {
	case !r.Allowed:
		color.New(color.FgRed, color.Bold).Printf("  🚫 BLOCKED: %s@%s\n", pkg, version)
	case len(r.Policy.Warn) > 0:
		color.Yellow("  ⚠  %s@%s — %d advisory", pkg, version, len(r.Policy.Warn))
	default:
		color.Green("  ✓  %s@%s — clean", pkg, version)
	}

	colorFor := func(msg string) *color.Color {
		switch {
		case strings.HasPrefix(msg, "[CVE]"):
			return color.New(color.FgRed)
		case strings.HasPrefix(msg, "[BEHAVIOUR]"):
			return color.New(color.FgMagenta)
		case strings.HasPrefix(msg, "[FRESHNESS]"):
			return color.New(color.FgCyan)
		case strings.HasPrefix(msg, "[TYPOSQUAT]"):
			return color.New(color.FgYellow)
		case strings.HasPrefix(msg, "[LICENCE]"):
			return color.New(color.FgYellow)
		default:
			return color.New(color.Faint)
		}
	}
	if !r.Allowed {
		for _, msg := range r.Policy.Deny {
			colorFor(msg).Printf("     • %s\n", msg)
		}
	}
	for _, msg := range r.Policy.Warn {
		colorFor(msg).Printf("     • %s\n", msg)
	}
	if r.Result.Meta.FilesScanned > 0 {
		color.New(color.Faint).Printf("     %d files scanned · %d behavioural rules\n",
			r.Result.Meta.FilesScanned, r.Result.Meta.RulesApplied)
	}
	if r.Cached {
		color.New(color.Faint).Println("     (cached result)")
	}
}

func parsePkgArg(arg string) (pkg, version string) {
	if strings.Contains(arg, "==") {
		parts := strings.SplitN(arg, "==", 2)
		return parts[0], parts[1]
	}
	if strings.HasPrefix(arg, "@") {
		// scoped: @scope/name or @scope/name@version
		rest := arg[1:]
		if i := strings.Index(rest, "@"); i >= 0 {
			return "@" + rest[:i], rest[i+1:]
		}
		return arg, "latest"
	}
	if i := strings.LastIndex(arg, "@"); i > 0 {
		return arg[:i], arg[i+1:]
	}
	return arg, "latest"
}

func historyCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent package install events",
		RunE: func(cmd *cobra.Command, args []string) error {
			banner()
			rows, err := db.History(limit)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				color.New(color.Faint).Println("  No installs recorded yet")
				return nil
			}
			fmt.Printf("  %-28s  %-14s  %-8s  %-10s  %s\n",
				color.CyanString("Package"),
				color.CyanString("Version"),
				color.CyanString("Eco"),
				color.CyanString("Action"),
				color.CyanString("Time"))
			fmt.Println("  " + strings.Repeat("─", 80))
			for _, r := range rows {
				var act *color.Color
				switch r.Action {
				case "blocked":
					act = color.New(color.FgRed)
				case "warned":
					act = color.New(color.FgYellow)
				case "bypassed":
					act = color.New(color.FgMagenta)
				default:
					act = color.New(color.FgGreen)
				}
				fmt.Printf("  %-28s  %-14s  %-8s  %s  %s\n",
					truncate(r.Pkg, 28),
					truncate(r.Version, 14),
					r.Ecosystem,
					act.Sprintf("%-10s", r.Action),
					color.New(color.Faint).Sprint(r.InstalledAt.Format("2006-01-02 15:04")))
			}
			fmt.Println()
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "number of entries")
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch [path]",
		Short: "Register a project for tracking (live watch needs daemon upgrade)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			} else {
				path, _ = os.Getwd()
			}
			if err := db.AddWatched(path); err != nil {
				return err
			}
			color.Green("  ✓ Tracking %s", path)
			color.New(color.Faint).Println("    Live watching requires daemon — see docs/upgrades/daemon.md")
			return nil
		},
	}
}

func allowCmd() *cobra.Command {
	var ecosystem, reason string
	cmd := &cobra.Command{
		Use:   "allow <package>",
		Short: "Allow a specific package past the security gate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, version := parsePkgArg(args[0])
			if version == "latest" {
				version = "*"
			}
			key := fmt.Sprintf("allow:%s:%s:%s", ecosystem, pkg, version)
			val := reason
			if val == "" {
				val = "manual bypass"
			}
			if err := db.SetConfig(key, val); err != nil {
				return err
			}
			color.Yellow("  ⚠  %s@%s added to allow list", pkg, version)
			if reason != "" {
				color.New(color.Faint).Printf("     Reason: %s\n", reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&ecosystem, "ecosystem", "e", "npm", "npm or pip")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "reason for the bypass")
	return cmd
}

func hooksCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "hooks <install|remove>",
		Short:     "Install or remove npm/yarn/pip hooks",
		ValidArgs: []string{"install", "remove"},
		Args:      cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "install":
				return hooks.Install()
			case "remove":
				return hooks.Remove()
			}
			return nil
		},
	}
}

func configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config [key] [value]",
		Short: "Get or set configuration values",
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				m, err := db.ListConfig()
				if err != nil {
					return err
				}
				for k, v := range m {
					if strings.HasPrefix(k, "allow:") {
						continue
					}
					fmt.Printf("  %s = %s\n", color.New(color.Faint).Sprint(k), color.WhiteString(v))
				}
			case 1:
				v := db.GetConfig(args[0])
				if v == "" {
					v = "(not set)"
				}
				fmt.Printf("  %s = %s\n", args[0], color.WhiteString(v))
			case 2:
				if err := db.SetConfig(args[0], args[1]); err != nil {
					return err
				}
				color.Green("  ✓ %s = %s", args[0], args[1])
			}
			return nil
		},
	}
}
