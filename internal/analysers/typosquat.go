// Package analysers contains the typosquat/freshness/behavioural detectors.
// typosquat.go: no network. Edit distance + known squats + homoglyphs.
package analysers

import (
	"strings"

	"github.com/satnambhatt/phlx/internal/policy"
)

// Known documented typosquats — subset, expand as needed.
var knownNpmSquats = map[string]string{
	"crossenv":      "cross-env",
	"cross-env.js":  "cross-env",
	"event-stream":  "event-stream",
	"flatmap-stream": "flatmap-stream",
	"jquery.js":     "jquery",
	"loadash":       "lodash",
	"lodahs":        "lodash",
	"node-fetchs":   "node-fetch",
	"react-dom-router": "react-router-dom",
	"requst":        "request",
	"reqeust":       "request",
	"discord.dll":   "discord.js",
	"twilio-npm":    "twilio",
	"ember-power-timepicker": "ember-power-select",
	"shrugging-logger": "winston",
	"colors-i18n":   "colors",
	"redux-react":   "react-redux",
	"axios-proxy":   "axios",
}

var knownPyPISquats = map[string]string{
	"python-sqlite":  "sqlite3",
	"djago":          "django",
	"djnago":         "django",
	"djangoo":        "django",
	"pythonkafka":    "kafka-python",
	"jeIlyfish":      "jellyfish",
	"colorsama":      "colorama",
	"colorma":        "colorama",
	"reqests":        "requests",
	"requesys":       "requests",
	"urllib":         "urllib3",
	"beautifulsoup":  "beautifulsoup4",
	"python-mysql":   "mysqlclient",
}

// Top packages used for edit-distance compare. Trimmed to demo set — extend.
var topNpm = []string{
	"react", "lodash", "express", "axios", "vue", "moment", "next", "webpack",
	"babel-core", "typescript", "chalk", "commander", "request", "uuid",
	"redux", "fs-extra", "mocha", "jest", "cross-env", "node-fetch",
	"semver", "yargs", "minimist", "left-pad", "dotenv", "prettier", "eslint",
	"jquery", "underscore", "rxjs", "tslib", "core-js", "regenerator-runtime",
	"debug", "ms", "mkdirp", "rimraf", "glob", "tar",
}

var topPyPI = []string{
	"requests", "urllib3", "numpy", "pandas", "django", "flask", "boto3",
	"pip", "setuptools", "wheel", "pytest", "six", "colorama", "click",
	"pyyaml", "cryptography", "certifi", "idna", "charset-normalizer",
	"jellyfish", "matplotlib", "scipy", "tensorflow", "torch",
}

var homoglyphPairs = [][2]string{
	{"o", "0"}, {"0", "o"},
	{"l", "1"}, {"1", "l"},
	{"i", "1"}, {"1", "i"},
	{"-", "_"}, {"_", "-"},
	{"rn", "m"}, {"m", "rn"},
}

func Typosquat(pkg, ecosystem string) []policy.Finding {
	pkg = strings.ToLower(pkg)
	var findings []policy.Finding

	known := knownNpmSquats
	top := topNpm
	if ecosystem == "pip" {
		known = knownPyPISquats
		top = topPyPI
	}

	if real, ok := known[pkg]; ok {
		findings = append(findings, policy.Finding{
			RuleID:      "T001",
			Severity:    policy.SeverityCritical,
			Description: "'" + pkg + "' is a documented typosquat of '" + real + "'",
			Source:      "typosquat",
		})
		return findings
	}

	// T002: edit distance — short names ≤1, long names ≤2
	maxDist := 2
	if len(pkg) <= 5 {
		maxDist = 1
	}
	for _, t := range top {
		if t == pkg {
			return findings
		}
		d := levenshtein(pkg, t)
		if d > 0 && d <= maxDist {
			findings = append(findings, policy.Finding{
				RuleID:      "T002",
				Severity:    policy.SeverityHigh,
				Description: "'" + pkg + "' is " + d2s(d) + " char away from '" + t + "'",
				Source:      "typosquat",
			})
			return findings
		}
	}

	// T003: homoglyph substitution
	for _, pair := range homoglyphPairs {
		swapped := strings.ReplaceAll(pkg, pair[0], pair[1])
		if swapped == pkg {
			continue
		}
		for _, t := range top {
			if swapped == t {
				findings = append(findings, policy.Finding{
					RuleID:      "T003",
					Severity:    policy.SeverityHigh,
					Description: "'" + pkg + "' is a homoglyph of '" + t + "'",
					Source:      "typosquat",
				})
				return findings
			}
		}
	}

	// T004: separator confusion
	alt := strings.ReplaceAll(strings.ReplaceAll(pkg, "_", "-"), "-", "_")
	if alt != pkg {
		for _, t := range top {
			if alt == t {
				findings = append(findings, policy.Finding{
					RuleID:      "T004",
					Severity:    policy.SeverityHigh,
					Description: "'" + pkg + "' differs from '" + t + "' only by separator",
					Source:      "typosquat",
				})
				return findings
			}
		}
	}

	return findings
}

func d2s(d int) string {
	if d == 1 {
		return "1"
	}
	return "2"
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
