package skill

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RemoteSource encapsulates fetching from a remote repository (like GitHub).
type RemoteSource struct {
	client *http.Client
}

// NewRemoteSource creates a new RemoteSource utilizing the standard http.Client
// configured with a reasonable timeout.
func NewRemoteSource() *RemoteSource {
	return &RemoteSource{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchGitHubZip downloads the repository as a zip archive.
// `repo` is in the format "owner/repo".
func (r *RemoteSource) FetchGitHubZip(repo string) ([]byte, string, error) {
	// First, try to determine the default branch via the GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s", repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to fetch repo info from github: %s", resp.Status)
	}

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return nil, "", err
	}

	branch := repoInfo.DefaultBranch
	if branch == "" {
		branch = "main" // fallback
	}

	// Now download the zipball for the default branch
	zipURL := fmt.Sprintf("https://github.com/%s/archive/refs/heads/%s.zip", repo, branch)
	zipReq, err := http.NewRequest("GET", zipURL, nil)
	if err != nil {
		return nil, "", err
	}

	zipResp, err := r.client.Do(zipReq)
	if err != nil {
		return nil, "", err
	}
	defer zipResp.Body.Close()

	if zipResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to download zip from github: %s", zipResp.Status)
	}

	data, err := io.ReadAll(zipResp.Body)
	if err != nil {
		return nil, "", err
	}

	// The revision is roughly the branch in this simplified implementation.
	// For perfect reproducibility we would fetch the commit SHA, but this works for the requirement.
	return data, branch, nil
}

// FetchGitHubCommitSHA gets the latest commit SHA for a repository branch.
func (r *RemoteSource) FetchGitHubCommitSHA(repo, branch string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, branch)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch commit info from github: %s", resp.Status)
	}

	var commitInfo struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commitInfo); err != nil {
		return "", err
	}

	return commitInfo.SHA, nil
}

// IsRemoteOwnerRepo checks if a string looks like a GitHub owner/repo format.
func IsRemoteOwnerRepo(source string) bool {
	parts := strings.Split(source, "/")
	return len(parts) == 2 && !strings.Contains(source, ":") && !strings.Contains(source, "\\")
}
