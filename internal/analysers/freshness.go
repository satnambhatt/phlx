package analysers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/satnambhatt/phlx/internal/policy"
)

type npmMetadata struct {
	Versions map[string]struct {
		Maintainers []struct{ Name string } `json:"maintainers"`
		Scripts     map[string]string       `json:"scripts"`
		Dist        struct {
			UnpackedSize int64 `json:"unpackedSize"`
			FileCount    int   `json:"fileCount"`
		} `json:"dist"`
	} `json:"versions"`
	Time     map[string]string `json:"time"`
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
}

type pypiMetadata struct {
	Info struct {
		Author       string `json:"author"`
		Version      string `json:"version"`
		AuthorEmail  string `json:"author_email"`
	} `json:"info"`
	Releases map[string][]struct {
		UploadTime string `json:"upload_time_iso_8601"`
		Size       int64  `json:"size"`
	} `json:"releases"`
}

func Freshness(pkg, version, ecosystem string) []policy.Finding {
	switch ecosystem {
	case "npm":
		return freshnessNpm(pkg, version)
	case "pip":
		return freshnessPypi(pkg, version)
	}
	return nil
}

func freshnessNpm(pkg, version string) []policy.Finding {
	var findings []policy.Finding
	url := fmt.Sprintf("https://registry.npmjs.org/%s", pkg)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return findings
	}
	defer resp.Body.Close()
	var meta npmMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return findings
	}

	if version == "" || version == "latest" {
		version = meta.DistTags.Latest
	}

	// F001: published < 48h
	if t, err := time.Parse(time.RFC3339, meta.Time[version]); err == nil {
		if time.Since(t) < 48*time.Hour {
			findings = append(findings, policy.Finding{
				RuleID: "F001", Severity: policy.SeverityHigh,
				Description: fmt.Sprintf("%s@%s published %.0fh ago", pkg, version, time.Since(t).Hours()),
				Source:      "freshness",
			})
		}
	}

	// F006: first ever release
	if len(meta.Versions) == 1 {
		findings = append(findings, policy.Finding{
			RuleID: "F006", Severity: policy.SeverityMedium,
			Description: fmt.Sprintf("%s first release", pkg), Source: "freshness",
		})
	}

	return findings
}

func freshnessPypi(pkg, version string) []policy.Finding {
	var findings []policy.Finding
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", pkg)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return findings
	}
	defer resp.Body.Close()
	var meta pypiMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return findings
	}

	if version == "" || version == "latest" {
		version = meta.Info.Version
	}

	if files, ok := meta.Releases[version]; ok && len(files) > 0 {
		if t, err := time.Parse(time.RFC3339, files[0].UploadTime); err == nil {
			if time.Since(t) < 48*time.Hour {
				findings = append(findings, policy.Finding{
					RuleID: "F001", Severity: policy.SeverityHigh,
					Description: fmt.Sprintf("%s==%s published %.0fh ago", pkg, version, time.Since(t).Hours()),
					Source:      "freshness",
				})
			}
		}
	}

	if len(meta.Releases) == 1 {
		findings = append(findings, policy.Finding{
			RuleID: "F006", Severity: policy.SeverityMedium,
			Description: fmt.Sprintf("%s first release", pkg), Source: "freshness",
		})
	}

	return findings
}
