package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var intakeSecretRe = regexp.MustCompile(`\b(tk_|sk_|sk-)[A-Za-z0-9]{20,}`)

func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError {
	if strings.TrimSpace(in.Story) == "" {
		return &IntakeError{ErrSchemaMismatch, "story is empty"}
	}
	if !contains(allowedCallers, in.Provenance.CallerID) {
		return &IntakeError{ErrUnauthorizedCaller, "caller " + in.Provenance.CallerID + " is not allowlisted"}
	}
	if in.Repo != "" {
		if e := checkRepo(in.Repo, trustedRoots); e != nil {
			return e
		}
	}
	if intakeSecretRe.MatchString(in.Story) {
		return &IntakeError{ErrSecretDetected, "story contains a token-like string"}
	}
	return nil
}

func checkRepo(repo string, roots []string) *IntakeError {
	if strings.Contains(repo, "://") {
		return &IntakeError{ErrPathOutsideRoots, "repo must be a local path"}
	}
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return &IntakeError{ErrPathOutsideRoots, "repo cannot be resolved: " + err.Error()}
	}
	for _, root := range roots {
		rr, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rr, resolved)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return &IntakeError{ErrPathOutsideRoots, "repo resolves outside the trusted roots"}
}

func loadCallers(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}
