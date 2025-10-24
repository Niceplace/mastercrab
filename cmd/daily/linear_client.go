package daily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/viper"
)

type LinearViewerRequest struct {
	Query         string `json:"query"`
	OperationName string `json:"operationName"`
}

type LinearViewerResponse struct {
	Data struct {
		LinearViewer
	} `json:"data"`
}

type LinearViewer struct {
	Viewer struct {
		AssignedIssues struct {
			Edges []struct {
				Node struct {
					ID    string `json:"id"`
					Title string `json:"title"`
					URL   string `json:"url"`
				}
			} `json:"edges"`
		} `json:"assignedIssues"`
	} `json:"viewer"`
}

// LinearIssueDetailsResponse represents the response from the issue details query
type LinearIssueDetailsResponse struct {
	Data struct {
		Issue LinearIssueDetails `json:"issue"`
	} `json:"data"`
}

// LinearIssueDetails contains detailed information about a single issue
type LinearIssueDetails struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Identifier  string `json:"identifier"`
	State       struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
	Priority      int    `json:"priority"`
	PriorityLabel string `json:"priorityLabel"`
	Labels        struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		Nodes []struct {
			ID        string `json:"id"`
			Body      string `json:"body"`
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
			User      struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"nodes"`
	} `json:"comments"`
	Assignee struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"assignee"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func GetViewerAssignedIssues(client *http.Client, date_filter string, config *viper.Viper) (LinearViewer, error) {
	// Get required config values
	linearAuth := config.GetString("linear.apiToken")
	baseURL := config.GetString("linear.baseURL")

	// Validate required config
	if baseURL == "" {
		return LinearViewer{}, fmt.Errorf("linear.baseURL is not configured")
	}
	if linearAuth == "" {
		return LinearViewer{}, fmt.Errorf("linear.apiToken is not configured")
	}

	// If date_filter is empty, default to the last 24 hours
	if date_filter == "" {
		date_filter = "-P1D"
	}

	// Build GraphQL request
	var operationName = "MyAssignedIssues"
	linearRequest := LinearViewerRequest{
		Query: fmt.Sprintf(`
			query %s {
				viewer {
					assignedIssues(filter: { updatedAt: { gte: "%s" }}) {
						edges {
							node {
								id title url
							}
						}
					}
				}
			}
		`, operationName, date_filter),
		OperationName: operationName,
	}

	jsonValue, err := json.Marshal(linearRequest)
	if err != nil {
		return LinearViewer{}, fmt.Errorf("failed to create JSON payload for GraphQL request: %w", err)
	}

	// Create HTTP request
	request, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return LinearViewer{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", linearAuth)

	// Execute request
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error querying Linear's API %s\n", err)
		return LinearViewer{}, err
	}

	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)

	var viewer LinearViewerResponse
	err = json.Unmarshal(data, &viewer)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON %s\n", err)
		return LinearViewer{}, err
	}

	return viewer.Data.LinearViewer, nil
}

// GetIssueDetails fetches detailed information for a single issue by ID
func GetIssueDetails(client *http.Client, issueID string, config *viper.Viper) (LinearIssueDetails, error) {
	// Get required config values
	linearAuth := config.GetString("linear.apiToken")
	baseURL := config.GetString("linear.baseURL")

	// Validate required config
	if baseURL == "" {
		return LinearIssueDetails{}, fmt.Errorf("linear.baseURL is not configured")
	}
	if linearAuth == "" {
		return LinearIssueDetails{}, fmt.Errorf("linear.apiToken is not configured")
	}
	if issueID == "" {
		return LinearIssueDetails{}, fmt.Errorf("issueID is required")
	}

	// Build GraphQL request for issue details
	var operationName = "GetIssueDetails"
	linearRequest := LinearViewerRequest{
		Query: fmt.Sprintf(`
			query %s {
				issue(id: "%s") {
					id
					title
					description
					url
					identifier
					state {
						name
						type
					}
					priority
					priorityLabel
					labels {
						nodes {
							name
							color
						}
					}
					comments {
						nodes {
							id
							body
							createdAt
							updatedAt
							user {
								name
							}
						}
					}
					assignee {
						name
						email
					}
					createdAt
					updatedAt
				}
			}
		`, operationName, issueID),
		OperationName: operationName,
	}

	jsonValue, err := json.Marshal(linearRequest)
	if err != nil {
		return LinearIssueDetails{}, fmt.Errorf("failed to create JSON payload for GraphQL request: %w", err)
	}

	// Create HTTP request
	request, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return LinearIssueDetails{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", linearAuth)

	// Execute request
	response, err := client.Do(request)
	if err != nil {
		return LinearIssueDetails{}, fmt.Errorf("error querying Linear's API: %w", err)
	}

	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)

	var issueResponse LinearIssueDetailsResponse
	err = json.Unmarshal(data, &issueResponse)
	if err != nil {
		return LinearIssueDetails{}, fmt.Errorf("error unmarshalling JSON: %w", err)
	}

	return issueResponse.Data.Issue, nil
}
