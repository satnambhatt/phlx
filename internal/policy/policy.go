// Package policy mirrors the inline policy from scanner/index.js.
// Source of truth when OPA is not running.
package policy

import "fmt"

type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

type Vuln struct {
	ID, PkgName, InstalledVersion, FixedVersion, Title, Source string
	Severity                                                   Severity
}

type License struct {
	Name, PkgName string
	Severity      Severity
}

type Finding struct {
	RuleID, Description, Source, File string
	Severity                          Severity
}

type Input struct {
	Pkg, Version, Ecosystem string
	Vulns                   []Vuln
	Licenses                []License
	Behavioural             []Finding
	Freshness               []Finding
	Typosquat               []Finding
}

type Result struct {
	Allow bool     `json:"allow"`
	Deny  []string `json:"deny"`
	Warn  []string `json:"warn"`
}

var blockedLic = map[string]bool{
	"GPL-2.0": true, "GPL-2.0-only": true,
	"GPL-3.0": true, "GPL-3.0-only": true,
	"AGPL-3.0": true, "AGPL-3.0-only": true,
	"SSPL-1.0": true,
}

var warnLic = map[string]bool{
	"LGPL-2.0": true, "LGPL-2.1": true, "LGPL-3.0": true,
	"MPL-2.0": true,
}

func Evaluate(in Input) Result {
	res := Result{Allow: true, Deny: []string{}, Warn: []string{}}

	for _, v := range in.Vulns {
		switch v.Severity {
		case SeverityCritical:
			res.Deny = append(res.Deny, fmt.Sprintf("[CVE] CRITICAL %s in %s", v.ID, v.PkgName))
		case SeverityHigh:
			if v.FixedVersion == "" {
				res.Deny = append(res.Deny, fmt.Sprintf("[CVE] HIGH %s in %s — no fix", v.ID, v.PkgName))
			} else {
				res.Warn = append(res.Warn, fmt.Sprintf("[CVE] HIGH %s fixable → upgrade to %s", v.ID, v.FixedVersion))
			}
		case SeverityMedium:
			res.Warn = append(res.Warn, fmt.Sprintf("[CVE] MEDIUM %s in %s", v.ID, v.PkgName))
		}
	}

	for _, l := range in.Licenses {
		if blockedLic[l.Name] {
			res.Deny = append(res.Deny, fmt.Sprintf("[LICENCE] %s not permitted in %s", l.Name, l.PkgName))
		} else if warnLic[l.Name] {
			res.Warn = append(res.Warn, fmt.Sprintf("[LICENCE] %s in %s — review", l.Name, l.PkgName))
		}
	}

	for _, f := range in.Behavioural {
		msg := fmt.Sprintf("[BEHAVIOUR] %s: %s", f.RuleID, f.Description)
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			res.Deny = append(res.Deny, msg)
		} else {
			res.Warn = append(res.Warn, msg)
		}
	}

	for _, f := range in.Freshness {
		msg := fmt.Sprintf("[FRESHNESS] %s: %s", f.RuleID, f.Description)
		if f.Severity == SeverityCritical {
			res.Deny = append(res.Deny, msg)
		} else {
			res.Warn = append(res.Warn, msg)
		}
	}

	for _, f := range in.Typosquat {
		msg := fmt.Sprintf("[TYPOSQUAT] %s: %s", f.RuleID, f.Description)
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			res.Deny = append(res.Deny, msg)
		} else {
			res.Warn = append(res.Warn, msg)
		}
	}

	res.Allow = len(res.Deny) == 0
	return res
}
