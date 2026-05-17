// Package registry talks to npm + PyPI to resolve versions and fetch tarballs.
package registry

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type NpmMeta struct {
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Versions map[string]struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	} `json:"versions"`
}

func ResolveNpmVersion(pkg, versionSpec string) (string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", pkg)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("npm: %s not found", pkg)
	}
	var meta NpmMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}
	if versionSpec == "" || versionSpec == "latest" {
		return meta.DistTags.Latest, nil
	}
	if _, ok := meta.Versions[versionSpec]; ok {
		return versionSpec, nil
	}
	return meta.DistTags.Latest, nil
}

// DownloadNpmTarball pulls and extracts npm tarball to a fresh tmpdir.
// Returns the directory containing extracted files.
func DownloadNpmTarball(pkg, version string) (string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s/%s", pkg, version)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("npm: %s@%s not found", pkg, version)
	}
	var meta struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}
	if meta.Dist.Tarball == "" {
		return "", errors.New("no tarball url")
	}

	dir, err := os.MkdirTemp("", "phalanx-dl-")
	if err != nil {
		return "", err
	}
	tr, err := httpClient.Get(meta.Dist.Tarball)
	if err != nil {
		return "", err
	}
	defer tr.Body.Close()

	if err := extractTarGz(tr.Body, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func extractTarGz(r io.Reader, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Strip leading "package/" path that npm tarballs always have.
		name := hdr.Name
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			continue
		}
		target := filepath.Join(dst, name)
		// Block path traversal
		if !strings.HasPrefix(target, dst) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
