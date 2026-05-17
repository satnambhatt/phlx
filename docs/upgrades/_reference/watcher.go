//go:build ignore
// +build ignore

// Reference scaffold for the optional file watcher.
// Not compiled into the main build. Copy into `internal/watcher/` and add
// `github.com/fsnotify/fsnotify` to go.mod when you wire this up.
//
// Watches package.json, requirements.txt, Pipfile, pyproject.toml across
// every project registered via `phalanx watch <path>`. On change: diffs old
// vs new dep list, runs scanner.Scan() for newly added or version-changed
// packages, then writes install events via db.RecordInstall().
//
// See ../daemon.md for the design notes.

package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/satnambhatt/phlx/internal/db"
	"github.com/satnambhatt/phlx/internal/scanner"
)

type Event struct {
	Name, Version, Ecosystem string
	Allowed                  bool
	Deny, Warn               []string
	Err                      error
}

type Dep struct {
	Name, Version, Ecosystem string
}

var (
	known  = map[string][]Dep{}
	knownM sync.Mutex
)

func Start(ctx context.Context, events chan<- Event) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	rows, err := db.DB.Query(`SELECT path FROM watched`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		paths = append(paths, p)
	}

	for _, p := range paths {
		// fsnotify is non-recursive — walk and add every subdir explicitly,
		// skipping node_modules / .git / .venv / dist / build.
		filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			base := d.Name()
			if base == "node_modules" || base == ".git" || base == ".venv" ||
				base == "venv" || base == "dist" || base == "build" ||
				base == "__pycache__" {
				return filepath.SkipDir
			}
			return w.Add(path)
		})
	}

	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				if !isManifest(ev.Name) {
					continue
				}
				go onChange(ev.Name, events)
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				events <- Event{Err: err}
			}
		}
	}()
	return nil
}

func isManifest(p string) bool {
	base := filepath.Base(p)
	return base == "package.json" || base == "requirements.txt" ||
		base == "Pipfile" || base == "pyproject.toml"
}

func onChange(path string, events chan<- Event) {
	newDeps := parseDeps(path)
	knownM.Lock()
	oldDeps := known[path]
	known[path] = newDeps
	knownM.Unlock()

	changed := diff(oldDeps, newDeps)
	for _, d := range changed {
		res, err := scanner.Scan(scanner.Request{
			Pkg: d.Name, Version: d.Version, Ecosystem: d.Ecosystem,
		})
		if err != nil {
			events <- Event{Name: d.Name, Version: d.Version, Ecosystem: d.Ecosystem, Err: err}
			continue
		}
		events <- Event{
			Name: d.Name, Version: d.Version, Ecosystem: d.Ecosystem,
			Allowed: res.Allowed, Deny: res.Policy.Deny, Warn: res.Policy.Warn,
		}
	}
}

func diff(old, neu []Dep) []Dep {
	idx := map[string]string{}
	for _, d := range old {
		idx[d.Name] = d.Version
	}
	var out []Dep
	for _, d := range neu {
		if prev, ok := idx[d.Name]; !ok || prev != d.Version {
			out = append(out, d)
		}
	}
	return out
}

var pyDepRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:[><=!~]+(.+))?`)

func parseDeps(path string) []Dep {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	base := filepath.Base(path)

	switch base {
	case "package.json":
		var pkg struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		_ = json.Unmarshal(data, &pkg)
		var out []Dep
		for n, v := range pkg.Dependencies {
			out = append(out, Dep{n, strings.TrimLeft(v, "^~>=<"), "npm"})
		}
		for n, v := range pkg.DevDependencies {
			out = append(out, Dep{n, strings.TrimLeft(v, "^~>=<"), "npm"})
		}
		return out

	case "requirements.txt":
		var out []Dep
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			m := pyDepRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			v := m[2]
			if v == "" {
				v = "latest"
			}
			out = append(out, Dep{m[1], v, "pip"})
		}
		return out
	}
	// TODO: Pipfile + pyproject.toml parsing
	return nil
}

func WatchProject(path string) error { return db.AddWatched(path) }

// Used by daemon to print compact log lines.
func FormatLine(e Event) string {
	if !e.Allowed {
		return fmt.Sprintf("[phalanx] BLOCKED %s@%s — %s", e.Name, e.Version, firstOr(e.Deny, ""))
	}
	if len(e.Warn) > 0 {
		return fmt.Sprintf("[phalanx] WARN    %s@%s — %s", e.Name, e.Version, e.Warn[0])
	}
	return fmt.Sprintf("[phalanx] OK      %s@%s", e.Name, e.Version)
}

func firstOr(s []string, fallback string) string {
	if len(s) > 0 {
		return s[0]
	}
	return fallback
}
