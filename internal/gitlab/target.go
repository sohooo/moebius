package gitlab

import (
	"fmt"
	"os"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
)

type Target struct {
	BaseURL         string
	ProjectID       string
	MergeRequestIID string
	JobToken        string
}

func ResolveTarget(opts cli.Options) (Target, error) {
	baseURL := firstNonEmpty(opts.GitLabBaseURL, os.Getenv("CI_API_V4_URL"))
	if baseURL == "" {
		if serverURL := os.Getenv("CI_SERVER_URL"); serverURL != "" {
			baseURL = strings.TrimRight(serverURL, "/") + "/api/v4"
		}
	}

	projectID := firstNonEmpty(opts.ProjectID, os.Getenv("CI_PROJECT_ID"))
	mrIID := firstNonEmpty(opts.MergeRequestIID, os.Getenv("CI_MERGE_REQUEST_IID"))
	jobToken := os.Getenv("CI_JOB_TOKEN")

	if projectID == "" {
		return Target{}, fmt.Errorf("missing GitLab project ID; set CI_PROJECT_ID or use --project-id")
	}
	if mrIID == "" {
		return Target{}, fmt.Errorf("missing GitLab merge request IID; set CI_MERGE_REQUEST_IID or use --mr-iid")
	}
	if baseURL == "" {
		return Target{}, fmt.Errorf("missing GitLab API base URL; set CI_API_V4_URL/CI_SERVER_URL or use --gitlab-base-url")
	}
	if jobToken == "" {
		return Target{}, fmt.Errorf("missing CI_JOB_TOKEN")
	}

	return Target{
		BaseURL:         baseURL,
		ProjectID:       projectID,
		MergeRequestIID: mrIID,
		JobToken:        jobToken,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
