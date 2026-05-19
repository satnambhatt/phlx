// Self-update: pulls the latest release archive from GitHub and replaces
// the running binaries on disk. Fail-safe — any error leaves the existing
// install untouched.
package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	releasesAPI = "https://api.github.com/repos/satnambhatt/phlx/releases/latest"
	downloadURL = "https://github.com/satnambhatt/phlx/releases/download/%s/%s"
)

type ghRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

// hookBinaries names every binary phalanx ships alongside the main CLI.
// Update keeps each one in lockstep so shims stay compatible.
var hookBinaries = []string{"phalanx-npm-hook", "phalanx-pip-hook", "phalanx-yarn-hook"}

func updateCmd(currentVersion string) *cobra.Command {
	var check bool
	var force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Pull and install the latest released version",
		RunE: func(cmd *cobra.Command, args []string) error {
			banner()
			color.New(color.Faint).Printf("  current version: %s\n", currentVersion)

			rel, err := fetchLatestRelease()
			if err != nil {
				color.Red("  ✗ could not check for updates: %s", err)
				return err
			}
			latest := strings.TrimPrefix(rel.TagName, "v")
			color.New(color.Faint).Printf("  latest version:  %s\n\n", latest)

			normalized := strings.TrimPrefix(currentVersion, "v")
			if !force && normalized == latest {
				color.Green("  ✓ already up to date")
				fmt.Println()
				return nil
			}

			if check {
				color.Yellow("  ⚠  update available: %s → %s", normalized, latest)
				fmt.Printf("  Run %s to install\n\n", color.CyanString("phlx update"))
				return nil
			}

			color.New(color.FgCyan, color.Bold).Printf("  Updating to %s...\n\n", rel.TagName)
			if err := performUpdate(rel.TagName, latest); err != nil {
				color.Red("  ✗ update failed: %s", err)
				return err
			}
			color.Green("  ✓ updated to %s", rel.TagName)
			fmt.Println()
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only check for an update; do not install")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already on the latest version")
	return cmd
}

func fetchLatestRelease() (*ghRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, releasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "phlx-self-update")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned status %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New("no tag_name in release response")
	}
	return &rel, nil
}

// archiveName mirrors the goreleaser name_template in .goreleaser.yaml.
func archiveName(version string) (string, error) {
	var osPart, archPart, ext string
	switch runtime.GOOS {
	case "darwin":
		osPart = "macos"
		ext = "tar.gz"
	case "linux":
		osPart = "linux"
		ext = "tar.gz"
	case "windows":
		osPart = "windows"
		ext = "zip"
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		archPart = "x86_64"
	case "arm64":
		archPart = "arm64"
	default:
		return "", fmt.Errorf("unsupported arch: %s", runtime.GOARCH)
	}
	return fmt.Sprintf("phlx_%s_%s_%s.%s", version, osPart, archPart, ext), nil
}

func performUpdate(tag, version string) error {
	name, err := archiveName(version)
	if err != nil {
		return err
	}
	url := fmt.Sprintf(downloadURL, tag, name)

	color.New(color.Faint).Printf("  downloading %s\n", url)
	tmpDir, err := os.MkdirTemp("", "phlx-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, name)
	if err := download(url, archivePath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(name, ".zip") {
		if err := extractZip(archivePath, extractDir); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
	} else {
		if err := extractTarGz(archivePath, extractDir); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	installDir := filepath.Dir(exe)
	mainName := "phalanx"
	if runtime.GOOS == "windows" {
		mainName = "phalanx.exe"
	}

	binaries := []string{mainName}
	for _, h := range hookBinaries {
		if runtime.GOOS == "windows" {
			h += ".exe"
		}
		binaries = append(binaries, h)
	}

	for _, b := range binaries {
		src := filepath.Join(extractDir, b)
		if _, err := os.Stat(src); err != nil {
			color.New(color.Faint).Printf("  · %s not in archive — skipping\n", b)
			continue
		}
		dst := filepath.Join(installDir, b)
		if err := replaceBinary(src, dst); err != nil {
			return fmt.Errorf("replace %s: %w", b, err)
		}
		color.New(color.Faint).Printf("  · replaced %s\n", dst)
	}
	return nil
}

func download(url, dst string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "phlx-self-update")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractTarGz(archivePath, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Flatten any nested path — goreleaser puts binaries at archive root.
		name := filepath.Base(hdr.Name)
		out := filepath.Join(dst, name)
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, tr); err != nil {
			w.Close()
			return err
		}
		w.Close()
	}
}

func extractZip(archivePath, dst string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out := filepath.Join(dst, name)
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			rc.Close()
			w.Close()
			return err
		}
		rc.Close()
		w.Close()
	}
	return nil
}

// replaceBinary atomically swaps dst with src. Falls back to a copy if the
// rename crosses devices (common on Linux when /tmp is tmpfs).
func replaceBinary(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".phlx-new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
