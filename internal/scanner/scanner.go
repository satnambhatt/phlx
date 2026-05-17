// Package scanner orchestrates the 5-stage pipeline.
package scanner

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/satnambhatt/phlx/internal/analysers"
	"github.com/satnambhatt/phlx/internal/db"
	"github.com/satnambhatt/phlx/internal/policy"
	"github.com/satnambhatt/phlx/internal/registry"
	"github.com/satnambhatt/phlx/internal/trivy"
)

const cacheTTL = 24 * time.Hour

type Request struct {
	Pkg, Version, Ecosystem string
}

type Meta struct {
	FilesScanned int `json:"filesScanned"`
	RulesApplied int `json:"rulesApplied"`
}

type ScanResult struct {
	Pkg, Version, Ecosystem string
	ScannedAt               time.Time
	Vulns                   []policy.Vuln
	Licenses                []policy.License
	Behavioural             []policy.Finding
	Freshness               []policy.Finding
	Typosquat               []policy.Finding
	Meta                    Meta
	EarlyExit               string
}

type FullResult struct {
	Allowed bool          `json:"allowed"`
	Cached  bool          `json:"cached"`
	Result  ScanResult    `json:"result"`
	Policy  policy.Result `json:"policy"`
}

func Scan(req Request) (FullResult, error) {
	if req.Ecosystem == "" {
		req.Ecosystem = "npm"
	}

	// Cache check
	if rJSON, pJSON, ok := db.GetCachedScan(req.Pkg, req.Version, req.Ecosystem, cacheTTL); ok {
		var sr ScanResult
		var pr policy.Result
		_ = json.Unmarshal([]byte(rJSON), &sr)
		_ = json.Unmarshal([]byte(pJSON), &pr)
		return FullResult{Allowed: pr.Allow, Cached: true, Result: sr, Policy: pr}, nil
	}

	sr := ScanResult{
		Pkg: req.Pkg, Version: req.Version, Ecosystem: req.Ecosystem,
		ScannedAt: time.Now(),
	}

	// Stage 1 — typosquat (no I/O)
	sr.Typosquat = analysers.Typosquat(req.Pkg, req.Ecosystem)

	// Early exit on CRITICAL squat — skip download
	for _, f := range sr.Typosquat {
		if f.Severity == policy.SeverityCritical {
			sr.EarlyExit = "typosquat"
			pr := policy.Evaluate(policy.Input{Typosquat: sr.Typosquat})
			persist(req, sr, pr)
			return FullResult{Allowed: false, Result: sr, Policy: pr}, nil
		}
	}

	// Stage 2 — freshness (one registry call)
	sr.Freshness = analysers.Freshness(req.Pkg, req.Version, req.Ecosystem)

	// Stage 3+4 — download tarball, then Trivy + behavioural in parallel
	dir, err := download(req)
	if err != nil {
		return FullResult{}, err
	}
	defer os.RemoveAll(dir)

	var (
		wg          sync.WaitGroup
		trivyOut    trivy.Output
		behaviouRes analysers.BehaviouralResult
	)
	wg.Add(2)
	go func() { defer wg.Done(); trivyOut = trivy.Scan(dir) }()
	go func() { defer wg.Done(); behaviouRes = analysers.Behavioural(dir) }()
	wg.Wait()

	sr.Vulns = trivyOut.Vulns
	sr.Licenses = trivyOut.Licenses
	sr.Behavioural = behaviouRes.Findings
	sr.Meta = Meta{FilesScanned: behaviouRes.FilesScanned, RulesApplied: behaviouRes.RulesApplied}

	// Stage 5 — policy
	pr := policy.Evaluate(policy.Input{
		Pkg: req.Pkg, Version: req.Version, Ecosystem: req.Ecosystem,
		Vulns: sr.Vulns, Licenses: sr.Licenses,
		Behavioural: sr.Behavioural,
		Freshness:   sr.Freshness,
		Typosquat:   sr.Typosquat,
	})
	persist(req, sr, pr)

	return FullResult{Allowed: pr.Allow, Result: sr, Policy: pr}, nil
}

func download(req Request) (string, error) {
	switch req.Ecosystem {
	case "npm":
		return registry.DownloadNpmTarball(req.Pkg, req.Version)
	case "pip":
		return registry.DownloadPypiPackage(req.Pkg, req.Version)
	}
	return "", errors.New("unsupported ecosystem")
}

func persist(req Request, sr ScanResult, pr policy.Result) {
	rb, _ := json.Marshal(sr)
	pb, _ := json.Marshal(pr)
	_ = db.StoreScan(req.Pkg, req.Version, req.Ecosystem, string(rb), string(pb))
}
