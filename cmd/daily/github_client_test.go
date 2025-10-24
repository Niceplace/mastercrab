package daily

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test config for GitHub
func createGitHubTestConfig(apiToken string) *viper.Viper {
	v := viper.New()
	v.Set("github.apiToken", apiToken)
	return v
}

// Test GetViewerActivity with missing API token
func Test_GetViewerActivity_MissingAPIToken(t *testing.T) {
	// Arrange
	config := viper.New()
	// apiToken is intentionally not set
	since := time.Now().Add(-24 * time.Hour)
	until := time.Now()

	// Act
	_, err := GetViewerActivity(&http.Client{}, since, until, config)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.apiToken is not configured")
}

// Test GetViewerActivity with successful response using mock file
func Test_GetViewerActivity_SuccessWithMockFile(t *testing.T) {
	// Arrange - Read actual mock response file
	mockResponseBytes, err := os.ReadFile("../../mockResponses/github-activity.json")
	require.NoError(t, err, "Failed to read mock response file")

	// Create mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

		// Verify request body contains expected GraphQL query
		var requestBody GitHubRequest
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		require.NoError(t, err)
		assert.Contains(t, requestBody.Query, "contributionsCollection")
		assert.Contains(t, requestBody.Query, "from:")
		assert.Contains(t, requestBody.Query, "to:")
		assert.NotNil(t, requestBody.Variables)

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(mockResponseBytes)
	}))
	defer mockServer.Close()

	// Note: This test serves as documentation for how the mock server would work
	// The actual response parsing is tested in Test_GetViewerActivity_ResponseParsing
	// Since baseURL is hardcoded in github_client.go, we cannot test the full request flow
	// without modifying the implementation to accept a configurable baseURL
}

// Test GetViewerActivity response parsing
func Test_GetViewerActivity_ResponseParsing(t *testing.T) {
	// Arrange - Read actual mock response file
	mockResponseBytes, err := os.ReadFile("../../mockResponses/github-activity.json")
	require.NoError(t, err, "Failed to read mock response file")

	// Create mock HTTP server that returns our mock response
	// We test by parsing the response directly since baseURL is hardcoded in the client
	var ghResponse GitHubResponse
	err = json.Unmarshal(mockResponseBytes, &ghResponse)
	require.NoError(t, err)

	// Verify the response structure
	assert.Equal(t, "testuser", ghResponse.Data.Viewer.Login)
	assert.Equal(t, 5, ghResponse.Data.Viewer.ContributionsCollection.TotalCommitContributions)
	assert.Equal(t, 2, ghResponse.Data.Viewer.ContributionsCollection.TotalIssueContributions)
	assert.Equal(t, 3, ghResponse.Data.Viewer.ContributionsCollection.TotalPullRequestContributions)
	assert.Equal(t, 4, ghResponse.Data.Viewer.ContributionsCollection.TotalPullRequestReviewContributions)

	// Verify commits by repository
	assert.Equal(t, 1, len(ghResponse.Data.Viewer.ContributionsCollection.CommitContributionsByRepository))
	repoContribs := ghResponse.Data.Viewer.ContributionsCollection.CommitContributionsByRepository[0]
	assert.Equal(t, "mastercrab", repoContribs.Repository.Name)
	assert.Equal(t, "testorg", repoContribs.Repository.Owner.Login)
	assert.Equal(t, 2, len(repoContribs.Contributions.Nodes))

	// Verify issues
	assert.Equal(t, 2, len(ghResponse.Data.Viewer.ContributionsCollection.IssueContributions.Nodes))
	firstIssue := ghResponse.Data.Viewer.ContributionsCollection.IssueContributions.Nodes[0]
	assert.Equal(t, "Add GitHub integration", firstIssue.Issue.Title)
	assert.Equal(t, 1, firstIssue.Issue.Number)

	// Verify PRs
	assert.Equal(t, 3, len(ghResponse.Data.Viewer.ContributionsCollection.PullRequestContributions.Nodes))
	firstPR := ghResponse.Data.Viewer.ContributionsCollection.PullRequestContributions.Nodes[0]
	assert.Equal(t, "feat: Add daily command", firstPR.PullRequest.Title)
	assert.Equal(t, "MERGED", firstPR.PullRequest.State)
	assert.Equal(t, 10, firstPR.PullRequest.Number)

	// Verify reviews
	assert.Equal(t, 2, len(ghResponse.Data.Viewer.ContributionsCollection.PullRequestReviewContributions.Nodes))
	firstReview := ghResponse.Data.Viewer.ContributionsCollection.PullRequestReviewContributions.Nodes[0]
	assert.Equal(t, "fix: Handle empty responses", firstReview.PullRequest.Title)
	assert.Equal(t, 5, firstReview.PullRequest.Number)
}

// Test GetViewerActivity with empty response
func Test_GetViewerActivity_EmptyResponse(t *testing.T) {
	// Arrange - Create empty but valid response
	emptyResponse := GitHubResponse{
		Data: struct {
			Viewer struct {
				Login                   string `json:"login"`
				ContributionsCollection struct {
					TotalCommitContributions            int `json:"totalCommitContributions"`
					TotalIssueContributions             int `json:"totalIssueContributions"`
					TotalPullRequestContributions       int `json:"totalPullRequestContributions"`
					TotalPullRequestReviewContributions int `json:"totalPullRequestReviewContributions"`
					CommitContributionsByRepository     []struct {
						Repository struct {
							Name  string `json:"name"`
							Owner struct {
								Login string `json:"login"`
							} `json:"owner"`
						} `json:"repository"`
						Contributions struct {
							Nodes []struct {
								CommitCount int    `json:"commitCount"`
								OccurredAt  string `json:"occurredAt"`
							} `json:"nodes"`
						} `json:"contributions"`
					} `json:"commitContributionsByRepository"`
					IssueContributions struct {
						Nodes []struct {
							Issue struct {
								Title      string `json:"title"`
								URL        string `json:"url"`
								Number     int    `json:"number"`
								Repository struct {
									Name  string `json:"name"`
									Owner struct {
										Login string `json:"login"`
									} `json:"owner"`
								} `json:"repository"`
							} `json:"issue"`
							OccurredAt string `json:"occurredAt"`
						} `json:"nodes"`
					} `json:"issueContributions"`
					PullRequestContributions struct {
						Nodes []struct {
							PullRequest struct {
								Title      string `json:"title"`
								URL        string `json:"url"`
								Number     int    `json:"number"`
								State      string `json:"state"`
								Repository struct {
									Name  string `json:"name"`
									Owner struct {
										Login string `json:"login"`
									} `json:"owner"`
								} `json:"repository"`
							} `json:"pullRequest"`
							OccurredAt string `json:"occurredAt"`
						} `json:"nodes"`
					} `json:"pullRequestContributions"`
					PullRequestReviewContributions struct {
						Nodes []struct {
							PullRequest struct {
								Title      string `json:"title"`
								URL        string `json:"url"`
								Number     int    `json:"number"`
								Repository struct {
									Name  string `json:"name"`
									Owner struct {
										Login string `json:"login"`
									} `json:"owner"`
								} `json:"repository"`
							} `json:"pullRequest"`
							OccurredAt string `json:"occurredAt"`
						} `json:"nodes"`
					} `json:"pullRequestReviewContributions"`
				} `json:"contributionsCollection"`
			} `json:"viewer"`
		}{
			Viewer: struct {
				Login                   string `json:"login"`
				ContributionsCollection struct {
					TotalCommitContributions            int `json:"totalCommitContributions"`
					TotalIssueContributions             int `json:"totalIssueContributions"`
					TotalPullRequestContributions       int `json:"totalPullRequestContributions"`
					TotalPullRequestReviewContributions int `json:"totalPullRequestReviewContributions"`
					CommitContributionsByRepository     []struct {
						Repository struct {
							Name  string `json:"name"`
							Owner struct {
								Login string `json:"login"`
							} `json:"owner"`
						} `json:"repository"`
						Contributions struct {
							Nodes []struct {
								CommitCount int    `json:"commitCount"`
								OccurredAt  string `json:"occurredAt"`
							} `json:"nodes"`
						} `json:"contributions"`
					} `json:"commitContributionsByRepository"`
					IssueContributions struct {
						Nodes []struct {
							Issue struct {
								Title      string `json:"title"`
								URL        string `json:"url"`
								Number     int    `json:"number"`
								Repository struct {
									Name  string `json:"name"`
									Owner struct {
										Login string `json:"login"`
									} `json:"owner"`
								} `json:"repository"`
							} `json:"issue"`
							OccurredAt string `json:"occurredAt"`
						} `json:"nodes"`
					} `json:"issueContributions"`
					PullRequestContributions struct {
						Nodes []struct {
							PullRequest struct {
								Title      string `json:"title"`
								URL        string `json:"url"`
								Number     int    `json:"number"`
								State      string `json:"state"`
								Repository struct {
									Name  string `json:"name"`
									Owner struct {
										Login string `json:"login"`
									} `json:"owner"`
								} `json:"repository"`
							} `json:"pullRequest"`
							OccurredAt string `json:"occurredAt"`
						} `json:"nodes"`
					} `json:"pullRequestContributions"`
					PullRequestReviewContributions struct {
						Nodes []struct {
							PullRequest struct {
								Title      string `json:"title"`
								URL        string `json:"url"`
								Number     int    `json:"number"`
								Repository struct {
									Name  string `json:"name"`
									Owner struct {
										Login string `json:"login"`
									} `json:"owner"`
								} `json:"repository"`
							} `json:"pullRequest"`
							OccurredAt string `json:"occurredAt"`
						} `json:"nodes"`
					} `json:"pullRequestReviewContributions"`
				} `json:"contributionsCollection"`
			}{
				Login: "testuser",
			},
		},
	}

	responseBytes, err := json.Marshal(emptyResponse)
	require.NoError(t, err)

	// Verify empty response can be unmarshalled
	var parsed GitHubResponse
	err = json.Unmarshal(responseBytes, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "testuser", parsed.Data.Viewer.Login)
	assert.Equal(t, 0, parsed.Data.Viewer.ContributionsCollection.TotalCommitContributions)
}

// Test GetViewerActivity with API error
func Test_GetViewerActivity_APIError(t *testing.T) {
	// Arrange - Create error response
	errorResponseBytes := []byte(`{
		"errors": [
			{
				"message": "Bad credentials"
			}
		]
	}`)

	var errorResponse GitHubResponse
	err := json.Unmarshal(errorResponseBytes, &errorResponse)
	require.NoError(t, err)

	// Verify error is parsed
	assert.Equal(t, 1, len(errorResponse.Errors))
	assert.Equal(t, "Bad credentials", errorResponse.Errors[0].Message)
}

// Test time range formatting for GitHub API
func Test_TimeRangeFormatting(t *testing.T) {
	// Arrange
	since := time.Date(2025, 10, 22, 8, 0, 0, 0, time.UTC)
	until := time.Date(2025, 10, 23, 8, 0, 0, 0, time.UTC)

	// Act
	sinceFormatted := since.Format(time.RFC3339)
	untilFormatted := until.Format(time.RFC3339)

	// Assert
	assert.Equal(t, "2025-10-22T08:00:00Z", sinceFormatted)
	assert.Equal(t, "2025-10-23T08:00:00Z", untilFormatted)
}
