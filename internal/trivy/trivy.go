// Package trivy invokes the Trivy CVE scanner.
// Tries local binary first, falls back to Docker image, skips on failure.
package trivy

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/satnambhatt/phlx/internal/policy"
)

type rawResult struct {
	Results []struct {
		Vulnerabilities []struct {
			VulnerabilityID  string
			Severity         string
			PkgName          string
			InstalledVersion string
			FixedVersion     string
			Title            string
		} `json:"Vulnerabilities"`
		Licenses []struct {
			Name     string
			PkgName  string
			Severity string
		} `json:"Licenses"`
	} `json:"Results"`
}

type Output struct {
	Vulns    []policy.Vuln
	Licenses []policy.License
	Skipped  bool
}

func Scan(targetDir string) Output {
	out := Output{}
	cacheDir := filepath.Join(os.Getenv("HOME"), ".phalanx", "trivy-cache")
	os.MkdirAll(cacheDir, 0o755)

	resultsFile := filepath.Join(os.TempDir(), "phalanx-trivy.json")
	defer os.Remove(resultsFile)

	cmd, err := build(targetDir, resultsFile, cacheDir)
	if err != nil {
		out.Skipped = true
		return out
	}
	if err := cmd.Run(); err != nil {
		// trivy exits non-zero when vulns found — try reading file anyway
	}

	data, err := os.ReadFile(resultsFile)
	if err != nil {
		out.Skipped = true
		return out
	}
	var raw rawResult
	if err := json.Unmarshal(data, &raw); err != nil {
		out.Skipped = true
		return out
	}

	for _, r := range raw.Results {
		for _, v := range r.Vulnerabilities {
			out.Vulns = append(out.Vulns, policy.Vuln{
				ID:               v.VulnerabilityID,
				Severity:         policy.Severity(v.Severity),
				PkgName:          v.PkgName,
				InstalledVersion: v.InstalledVersion,
				FixedVersion:     v.FixedVersion,
				Title:            v.Title,
				Source:           "trivy",
			})
		}
		for _, l := range r.Licenses {
			out.Licenses = append(out.Licenses, policy.License{
				Name: l.Name, PkgName: l.PkgName, Severity: policy.Severity(l.Severity),
			})
		}
	}
	return out
}

func build(targetDir, resultsFile, cacheDir string) (*exec.Cmd, error) {
	if _, err := exec.LookPath("trivy"); err == nil {
		return exec.Command(
			"trivy", "fs", targetDir,
			"--format", "json",
			"--output", resultsFile,
			"--scanners", "vuln,license",
			"--cache-dir", cacheDir,
			"--quiet",
		), nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return exec.Command(
			"docker", "run", "--rm",
			"-v", targetDir+":/scan:ro",
			"-v", cacheDir+":/root/.cache/trivy",
			"-v", filepath.Dir(resultsFile)+":/out",
			"aquasec/trivy:latest", "fs", "/scan",
			"--format", "json",
			"--output", "/out/"+filepath.Base(resultsFile),
			"--scanners", "vuln,license",
			"--quiet",
		), nil
	}
	return nil, errors.New("trivy not available")
}
