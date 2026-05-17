package analysers

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/satnambhatt/phlx/internal/policy"
)

const maxFileSize = 2 * 1024 * 1024 // 2 MB

var scanExts = map[string]bool{
	".js": true, ".mjs": true, ".cjs": true, ".ts": true,
	".json": true, ".py": true, ".sh": true,
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "__pycache__": true,
}

type rule struct {
	id       string
	severity policy.Severity
	desc     string
	match    *regexp.Regexp
	inJSON   bool // limit to package.json scripts section
}

var rules = []rule{
	{id: "B001", severity: policy.SeverityHigh, desc: "install script executes shell commands",
		match: regexp.MustCompile(`(?i)"(preinstall|postinstall|install)"\s*:\s*"[^"]*(curl|wget|bash|sh\s)`), inJSON: true},
	{id: "B002", severity: policy.SeverityCritical, desc: "install downloads and pipes to shell",
		match: regexp.MustCompile(`(?i)(curl|wget)[^"]*\|[^"]*(bash|sh)\b`)},
	{id: "B003", severity: policy.SeverityHigh, desc: "base64 decode + eval/exec",
		match: regexp.MustCompile(`(?s)(atob|Buffer\.from|base64\.b64decode)[^;]{0,200}(eval|Function|exec)`)},
	{id: "B004", severity: policy.SeverityMedium, desc: "large base64 string literal",
		match: regexp.MustCompile(`['"][A-Za-z0-9+/=]{300,}['"]`)},
	{id: "B005", severity: policy.SeverityHigh, desc: "eval or Function with dynamic args",
		match: regexp.MustCompile(`(?:^|\s)(eval|new\s+Function)\s*\(`)},
	{id: "B006", severity: policy.SeverityMedium, desc: "String.fromCharCode chain",
		match: regexp.MustCompile(`(String\.fromCharCode\([^)]*\)[^.]*){3,}`)},
	{id: "B007", severity: policy.SeverityHigh, desc: "install-time outbound HTTP",
		match: regexp.MustCompile(`https?://[^\s'"]+\.(ru|tk|top|cn|cc|click|xyz)\b`)},
	{id: "B009", severity: policy.SeverityCritical, desc: "reads SSH/AWS/passwd credentials",
		match: regexp.MustCompile(`\.ssh/|\.aws/credentials|/etc/passwd|\.npmrc|\.docker/config`)},
	{id: "B010", severity: policy.SeverityHigh, desc: "accesses credential env vars",
		match: regexp.MustCompile(`(AWS_SECRET|AWS_ACCESS_KEY|GITHUB_TOKEN|NPM_TOKEN|GH_TOKEN|DOCKER_PASSWORD)`)},
	{id: "B012", severity: policy.SeverityCritical, desc: "reverse shell pattern",
		match: regexp.MustCompile(`/dev/tcp/|nc\s+-e\s+(/bin/)?(bash|sh)`)},
	{id: "B013", severity: policy.SeverityHigh, desc: "writes to cron or shell rc",
		match: regexp.MustCompile(`(/etc/cron|\.bashrc|\.zshrc|\.profile)`)},
	{id: "B014", severity: policy.SeverityCritical, desc: "crypto mining pool",
		match: regexp.MustCompile(`stratum\+tcp://|cryptonight|xmrig|coinhive`)},
	{id: "B015", severity: policy.SeverityHigh, desc: "child_process exec/spawn in install context",
		match: regexp.MustCompile(`child_process\.(exec|spawn|execSync)`), inJSON: false},
	{id: "B016", severity: policy.SeverityHigh, desc: "reads parent ../../package.json (dep confusion)",
		match: regexp.MustCompile(`\.\./\.\./package\.json`)},
}

type BehaviouralResult struct {
	Findings     []policy.Finding
	FilesScanned int
	RulesApplied int
}

func Behavioural(targetDir string) BehaviouralResult {
	res := BehaviouralResult{RulesApplied: len(rules)}
	seen := map[string]bool{}

	filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !scanExts[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxFileSize {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		res.FilesScanned++
		text := string(data)
		isJSON := ext == ".json"
		rel, _ := filepath.Rel(targetDir, path)

		for _, r := range rules {
			if r.inJSON && !isJSON {
				continue
			}
			if r.match.MatchString(text) {
				key := r.id + "::" + rel
				if seen[key] {
					continue
				}
				seen[key] = true
				res.Findings = append(res.Findings, policy.Finding{
					RuleID: r.id, Severity: r.severity,
					Description: r.desc, Source: "behavioural", File: rel,
				})
			}
		}
		return nil
	})
	return res
}
