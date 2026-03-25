// Package codegen generates Go parser source files and test files from approved
// PendingRule records, then opens a GitHub pull request via the gh CLI.
//
// This is a stub implementation. The full implementation will be provided by
// Task 9 (parallel task). The stub satisfies the import requirements so that
// Task 10 CLI commands can be compiled and tested independently.
package codegen

import (
	"fmt"
	"strings"

	"github.com/xavierli/nethelper/internal/store"
)

// GeneratorOptions controls code generation behaviour.
type GeneratorOptions struct {
	// RepoRoot is the absolute path to the repository root on disk.
	RepoRoot string
	// ApprovedBy is the username recorded in the generated file header.
	ApprovedBy string
	// DryRun skips writing files and creating the PR when true.
	DryRun bool
}

// Generate writes parser source + test files for rule and opens a PR.
// It returns the PR URL on success.
func Generate(rule store.PendingRule, testCases []store.RuleTestCase, opts GeneratorOptions) (string, error) {
	if opts.DryRun {
		return "", nil
	}
	// Stub: real implementation provided by Task 9.
	return fmt.Sprintf("https://github.com/placeholder/pr/%d", rule.ID), nil
}

// GenerateParserFile returns the Go source text for the parser file that would
// be written for rule. Used for diff preview in 'nethelper rule regen --force'.
func GenerateParserFile(rule store.PendingRule) (string, error) {
	// Stub: real implementation provided by Task 9.
	return fmt.Sprintf("// parser stub for vendor=%s command=%s\npackage stub\n",
		rule.Vendor, rule.CommandPattern), nil
}

// TargetFilePath returns the repo-relative path where the generated parser file
// will be written for the given vendor and command pattern.
func TargetFilePath(vendor, commandPattern string) string {
	safe := strings.NewReplacer(" ", "_", "/", "_", "*", "x", "?", "x").Replace(commandPattern)
	safe = strings.ToLower(safe)
	return fmt.Sprintf("internal/parser/%s/generated_%s.go", vendor, safe)
}
