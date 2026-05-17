package registry

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PypiMeta struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Releases map[string][]struct {
		URL         string `json:"url"`
		Filename    string `json:"filename"`
		Packagetype string `json:"packagetype"`
	} `json:"releases"`
	URLs []struct {
		URL         string `json:"url"`
		Filename    string `json:"filename"`
		Packagetype string `json:"packagetype"`
	} `json:"urls"`
}

func ResolvePypiVersion(pkg, versionSpec string) (string, error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://pypi.org/pypi/%s/json", pkg))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("pypi: %s not found", pkg)
	}
	var meta PypiMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}
	if versionSpec == "" || versionSpec == "latest" {
		return meta.Info.Version, nil
	}
	if _, ok := meta.Releases[versionSpec]; ok {
		return versionSpec, nil
	}
	return meta.Info.Version, nil
}

// DownloadPypiPackage fetches a wheel/sdist and extracts to a tmpdir.
func DownloadPypiPackage(pkg, version string) (string, error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://pypi.org/pypi/%s/%s/json", pkg, version))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("pypi: %s==%s not found", pkg, version)
	}
	var meta PypiMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}

	var pick struct{ URL, Filename, Packagetype string }
	for _, u := range meta.URLs {
		if u.Packagetype == "bdist_wheel" {
			pick.URL, pick.Filename, pick.Packagetype = u.URL, u.Filename, u.Packagetype
			break
		}
	}
	if pick.URL == "" && len(meta.URLs) > 0 {
		pick.URL = meta.URLs[0].URL
		pick.Filename = meta.URLs[0].Filename
		pick.Packagetype = meta.URLs[0].Packagetype
	}
	if pick.URL == "" {
		return "", fmt.Errorf("no download urls for %s==%s", pkg, version)
	}

	dir, err := os.MkdirTemp("", "phalanx-dl-")
	if err != nil {
		return "", err
	}
	dl, err := httpClient.Get(pick.URL)
	if err != nil {
		return "", err
	}
	defer dl.Body.Close()

	if strings.HasSuffix(pick.Filename, ".whl") || strings.HasSuffix(pick.Filename, ".zip") {
		tmp := filepath.Join(dir, pick.Filename)
		f, err := os.Create(tmp)
		if err != nil {
			return "", err
		}
		io.Copy(f, dl.Body)
		f.Close()
		if err := extractZip(tmp, dir); err != nil {
			return "", err
		}
		os.Remove(tmp)
		return dir, nil
	}
	if strings.HasSuffix(pick.Filename, ".tar.gz") || strings.HasSuffix(pick.Filename, ".tgz") {
		if err := extractTarGz(dl.Body, dir); err != nil {
			return "", err
		}
		return dir, nil
	}
	// Save raw if format unknown; behavioural still scans
	tmp := filepath.Join(dir, pick.Filename)
	f, _ := os.Create(tmp)
	io.Copy(f, dl.Body)
	f.Close()
	return dir, nil
}

func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(target, dst) {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		io.Copy(out, rc)
		rc.Close()
		out.Close()
	}
	return nil
}
