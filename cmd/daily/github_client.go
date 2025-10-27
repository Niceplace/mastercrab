package daily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/viper"
)

// GitHubRequest represents a GraphQL request to GitHub's API
type GitHubRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GitHubResponse represents the response from GitHub's GraphQL API
type GitHubResponse struct {
	Data struct {
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
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// GitHubActivity represents aggregated GitHub activity for the viewer
type GitHubActivity struct {
	Username             string
	TotalCommits         int
	TotalIssues          int
	TotalPullRequests    int
	TotalReviews         int
	CommitsByRepo        map[string]int
	IssuesCreated        []GitHubIssue
	PullRequestsCreated  []GitHubPullRequest
	PullRequestsReviewed []GitHubPullRequest
}

type GitHubIssue struct {
	Title      string
	URL        string
	Number     int
	RepoName   string
	RepoOwner  string
	OccurredAt string
}

type GitHubPullRequest struct {
	Title      string
	URL        string
	Number     int
	State      string
	RepoName   string
	RepoOwner  string
	OccurredAt string
}

// GetViewerActivity fetches GitHub activity for the authenticated user within a time period
func GetViewerActivity(client *http.Client, since time.Time, until time.Time, config *viper.Viper) (GitHubActivity, error) {
	// Get required config values
	githubToken := config.GetString("github.apiToken")
	baseURL := "https://api.github.com/graphql"

	// Validate required config
	if githubToken == "" {
		return GitHubActivity{}, fmt.Errorf("github.apiToken is not configured")
	}

	// Build GraphQL query
	query := `
		query($from: DateTime!, $to: DateTime!) {
			viewer {
				login
				contributionsCollection(from: $from, to: $to) {
					totalCommitContributions
					totalIssueContributions
					totalPullRequestContributions
					totalPullRequestReviewContributions
					commitContributionsByRepository {
						repository {
							name
							owner {
								login
							}
						}
						contributions(first: 100) {
							nodes {
								commitCount
								occurredAt
							}
						}
					}
					issueContributions(first: 100) {
						nodes {
							issue {
								title
								url
								number
								repository {
									name
									owner {
										login
									}
								}
							}
							occurredAt
						}
					}
					pullRequestContributions(first: 100) {
						nodes {
							pullRequest {
								title
								url
								number
								state
								repository {
									name
									owner {
										login
									}
								}
							}
							occurredAt
						}
					}
					pullRequestReviewContributions(first: 100) {
						nodes {
							pullRequest {
								title
								url
								number
								repository {
									name
									owner {
										login
									}
								}
							}
							occurredAt
						}
					}
				}
			}
		}
	`

	// Create request with variables
	githubRequest := GitHubRequest{
		Query: query,
		Variables: map[string]interface{}{
			"from": since.Format(time.RFC3339),
			"to":   until.Format(time.RFC3339),
		},
	}

	jsonValue, err := json.Marshal(githubRequest)
	if err != nil {
		return GitHubActivity{}, fmt.Errorf("failed to create JSON payload for GraphQL request: %w", err)
	}

	// Create HTTP request
	request, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return GitHubActivity{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", githubToken))

	// Execute request
	response, err := client.Do(request)
	if err != nil {
		return GitHubActivity{}, fmt.Errorf("error querying GitHub's API: %w", err)
	}

	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return GitHubActivity{}, fmt.Errorf("error reading response body: %w", err)
	}

	var ghResponse GitHubResponse
	err = json.Unmarshal(data, &ghResponse)
	if err != nil {
		return GitHubActivity{}, fmt.Errorf("error unmarshalling JSON: %w", err)
	}

	// Check for GraphQL errors
	if len(ghResponse.Errors) > 0 {
		return GitHubActivity{}, fmt.Errorf("GitHub API error: %s", ghResponse.Errors[0].Message)
	}

	// Aggregate the data into a structured format
	activity := GitHubActivity{
		Username:          ghResponse.Data.Viewer.Login,
		TotalCommits:      ghResponse.Data.Viewer.ContributionsCollection.TotalCommitContributions,
		TotalIssues:       ghResponse.Data.Viewer.ContributionsCollection.TotalIssueContributions,
		TotalPullRequests: ghResponse.Data.Viewer.ContributionsCollection.TotalPullRequestContributions,
		TotalReviews:      ghResponse.Data.Viewer.ContributionsCollection.TotalPullRequestReviewContributions,
		CommitsByRepo:     make(map[string]int),
	}

	// Aggregate commits by repository
	for _, repoContrib := range ghResponse.Data.Viewer.ContributionsCollection.CommitContributionsByRepository {
		repoKey := fmt.Sprintf("%s/%s", repoContrib.Repository.Owner.Login, repoContrib.Repository.Name)
		totalCommits := 0
		for _, contrib := range repoContrib.Contributions.Nodes {
			totalCommits += contrib.CommitCount
		}
		activity.CommitsByRepo[repoKey] = totalCommits
	}

	// Extract issues created
	for _, issueContrib := range ghResponse.Data.Viewer.ContributionsCollection.IssueContributions.Nodes {
		activity.IssuesCreated = append(activity.IssuesCreated, GitHubIssue{
			Title:      issueContrib.Issue.Title,
			URL:        issueContrib.Issue.URL,
			Number:     issueContrib.Issue.Number,
			RepoName:   issueContrib.Issue.Repository.Name,
			RepoOwner:  issueContrib.Issue.Repository.Owner.Login,
			OccurredAt: issueContrib.OccurredAt,
		})
	}

	// Extract PRs created
	for _, prContrib := range ghResponse.Data.Viewer.ContributionsCollection.PullRequestContributions.Nodes {
		activity.PullRequestsCreated = append(activity.PullRequestsCreated, GitHubPullRequest{
			Title:      prContrib.PullRequest.Title,
			URL:        prContrib.PullRequest.URL,
			Number:     prContrib.PullRequest.Number,
			State:      prContrib.PullRequest.State,
			RepoName:   prContrib.PullRequest.Repository.Name,
			RepoOwner:  prContrib.PullRequest.Repository.Owner.Login,
			OccurredAt: prContrib.OccurredAt,
		})
	}

	// Extract PR reviews
	for _, reviewContrib := range ghResponse.Data.Viewer.ContributionsCollection.PullRequestReviewContributions.Nodes {
		activity.PullRequestsReviewed = append(activity.PullRequestsReviewed, GitHubPullRequest{
			Title:      reviewContrib.PullRequest.Title,
			URL:        reviewContrib.PullRequest.URL,
			Number:     reviewContrib.PullRequest.Number,
			RepoName:   reviewContrib.PullRequest.Repository.Name,
			RepoOwner:  reviewContrib.PullRequest.Repository.Owner.Login,
			OccurredAt: reviewContrib.OccurredAt,
		})
	}

	return activity, nil
}
