package securitywatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var sourceCommitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var sourceBranchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// Source tracking is explicitly selected by the operator. A branch match says
// nothing about build provenance, release approval or image vulnerabilities.
func (c *Checker) sourceRelease(ctx context.Context, repo, installed string) Check {
	result := Check{Status: "unknown", Installed: c.SourceCommit, SourceBranch: c.SourceBranch}
	if installed != "dev" || !sourceCommitPattern.MatchString(c.SourceCommit) {
		result.Code = "invalid_source_build_metadata"
		return result
	}
	if !repositoryPattern.MatchString(repo) || strings.Contains(repo, "..") ||
		!sourceBranchPattern.MatchString(c.SourceBranch) || strings.Contains(c.SourceBranch, "..") ||
		strings.Contains(c.SourceBranch, "//") || strings.HasSuffix(c.SourceBranch, "/") ||
		strings.HasSuffix(c.SourceBranch, ".") || strings.HasSuffix(c.SourceBranch, ".lock") {
		result.Code = "invalid_source_tracking_target"
		return result
	}
	var ref struct {
		Ref    string `json:"ref"`
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	if err := c.sourceJSON(ctx, "/repos/"+repo+"/git/ref/heads/"+url.PathEscape(c.SourceBranch), &ref); err != nil {
		result.Code = "source_branch_lookup_unavailable"
		return result
	}
	if ref.Ref != "refs/heads/"+c.SourceBranch || ref.Object.Type != "commit" || !sourceCommitPattern.MatchString(ref.Object.SHA) {
		result.Code = "invalid_source_branch_response"
		return result
	}
	result.Latest = ref.Object.SHA
	if result.Installed == result.Latest {
		result.Status, result.Code = "up_to_date", "source_commit_current"
		return result
	}
	var comparison struct {
		Status     string `json:"status"`
		AheadBy    int    `json:"ahead_by"`
		BehindBy   int    `json:"behind_by"`
		BaseCommit struct {
			SHA string `json:"sha"`
		} `json:"base_commit"`
		MergeBaseCommit struct {
			SHA string `json:"sha"`
		} `json:"merge_base_commit"`
	}
	// Both sides are immutable commits, so a branch move during this request
	// cannot silently change the comparison target.
	if err := c.sourceJSON(ctx, "/repos/"+repo+"/compare/"+result.Installed+"..."+result.Latest, &comparison); err != nil {
		result.Code = "source_comparison_unavailable"
		return result
	}
	if comparison.BaseCommit.SHA != result.Installed || comparison.MergeBaseCommit.SHA != result.Installed ||
		comparison.Status != "ahead" || comparison.AheadBy <= 0 || comparison.BehindBy != 0 {
		result.Code = "source_history_not_fast_forward"
		return result
	}
	result.Status, result.Code = "update_available", "source_commit_update_available"
	return result
}

func (c *Checker) sourceJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "FleetDeck-security-watch")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("source metadata unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return fmt.Errorf("invalid source metadata")
	}
	return json.Unmarshal(data, target)
}
