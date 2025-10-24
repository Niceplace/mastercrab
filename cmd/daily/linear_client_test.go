package daily

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test config
func createTestConfig(baseURL, apiToken string) *viper.Viper {
	v := viper.New()
	v.Set("linear.baseURL", baseURL)
	v.Set("linear.apiToken", apiToken)
	return v
}

// Test GetViewerAssignedIssues returns error when baseURL is missing
func Test_GetViewerAssignedIssues_MissingBaseURL(t *testing.T) {
	// Arrange
	config := viper.New()
	config.Set("linear.apiToken", "test-token")
	// baseURL is intentionally not set

	// Act
	_, err := GetViewerAssignedIssues(&http.Client{}, "-P1D", config)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linear.baseURL is not configured")
}

// Test GetViewerAssignedIssues returns error when apiToken is missing
func Test_GetViewerAssignedIssues_MissingAPIToken(t *testing.T) {
	// Arrange
	config := viper.New()
	config.Set("linear.baseURL", "https://api.linear.app/graphql")
	// apiToken is intentionally not set

	// Act
	_, err := GetViewerAssignedIssues(&http.Client{}, "-P1D", config)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linear.apiToken is not configured")
}

// Test GetViewerAssignedIssues with successful response using mock file
func Test_GetViewerAssignedIssues_SuccessWithMockFile(t *testing.T) {
	// Arrange - Read actual mock response file
	mockResponseBytes, err := os.ReadFile("../../mockResponses/response.json")
	require.NoError(t, err, "Failed to read mock response file")

	// Create mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-token", r.Header.Get("Authorization"))

		// Verify request body contains expected GraphQL query
		var requestBody LinearViewerRequest
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		require.NoError(t, err)
		assert.Equal(t, "MyAssignedIssues", requestBody.OperationName)
		assert.Contains(t, requestBody.Query, "assignedIssues")
		assert.Contains(t, requestBody.Query, "-P1D")

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(mockResponseBytes)
	}))
	defer mockServer.Close()

	// Setup config with mock server URL
	config := createTestConfig(mockServer.URL, "test-api-token")

	// Act
	result, err := GetViewerAssignedIssues(&http.Client{}, "-P1D", config)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result.Viewer.AssignedIssues.Edges)
	assert.Equal(t, 2, len(result.Viewer.AssignedIssues.Edges))

	// Verify specific issue details
	firstIssue := result.Viewer.AssignedIssues.Edges[0].Node
	assert.Equal(t, "test-issue-id-001", firstIssue.ID)
	assert.Equal(t, "Implement user authentication feature", firstIssue.Title)
	assert.Contains(t, firstIssue.URL, "linear.app")
}

// Test GetViewerAssignedIssues with empty response
func Test_GetViewerAssignedIssues_EmptyResponse(t *testing.T) {
	// Arrange - Create mock server returning empty edges
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		emptyResponse := LinearViewerResponse{
			Data: struct{ LinearViewer }{
				LinearViewer: LinearViewer{
					Viewer: struct {
						AssignedIssues struct {
							Edges []struct {
								Node struct {
									ID    string `json:"id"`
									Title string `json:"title"`
									URL   string `json:"url"`
								}
							} `json:"edges"`
						} `json:"assignedIssues"`
					}{
						AssignedIssues: struct {
							Edges []struct {
								Node struct {
									ID    string `json:"id"`
									Title string `json:"title"`
									URL   string `json:"url"`
								}
							} `json:"edges"`
						}{
							Edges: []struct {
								Node struct {
									ID    string `json:"id"`
									Title string `json:"title"`
									URL   string `json:"url"`
								}
							}{},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(emptyResponse)
	}))
	defer mockServer.Close()

	config := createTestConfig(mockServer.URL, "test-token")

	// Act
	result, err := GetViewerAssignedIssues(&http.Client{}, "-P1D", config)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result.Viewer.AssignedIssues.Edges)
	assert.Equal(t, 0, len(result.Viewer.AssignedIssues.Edges))
}

// Test GetViewerAssignedIssues with API error
func Test_GetViewerAssignedIssues_APIError(t *testing.T) {
	// Arrange - Create mock server returning auth error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorResponse := map[string]interface{}{
			"errors": []map[string]interface{}{
				{
					"message": "Authentication required",
					"extensions": map[string]interface{}{
						"code": "AUTHENTICATION_ERROR",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(errorResponse)
	}))
	defer mockServer.Close()

	config := createTestConfig(mockServer.URL, "invalid-token")

	// Act
	result, err := GetViewerAssignedIssues(&http.Client{}, "-P1D", config)

	// Assert
	require.NoError(t, err) // API returns 401 but HTTP request succeeds
	// The result will have nil edges because the error response doesn't match our struct
	assert.Nil(t, result.Viewer.AssignedIssues.Edges)
}

// Test GetViewerAssignedIssues with custom date filter
func Test_GetViewerAssignedIssues_CustomDateFilter(t *testing.T) {
	// Arrange
	var capturedQuery string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody LinearViewerRequest
		json.NewDecoder(r.Body).Decode(&requestBody)
		capturedQuery = requestBody.Query

		// Return minimal valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LinearViewerResponse{})
	}))
	defer mockServer.Close()

	config := createTestConfig(mockServer.URL, "test-token")

	// Act
	_, err := GetViewerAssignedIssues(&http.Client{}, "-P7D", config)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "-P7D", "Query should contain custom date filter")
}

// Test GetViewerAssignedIssues with empty date filter (uses default)
func Test_GetViewerAssignedIssues_DefaultDateFilter(t *testing.T) {
	// Arrange
	var capturedQuery string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody LinearViewerRequest
		json.NewDecoder(r.Body).Decode(&requestBody)
		capturedQuery = requestBody.Query

		// Return minimal valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LinearViewerResponse{})
	}))
	defer mockServer.Close()

	config := createTestConfig(mockServer.URL, "test-token")

	// Act - pass empty string for date filter
	_, err := GetViewerAssignedIssues(&http.Client{}, "", config)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "-P1D", "Query should contain default date filter -P1D")
}

// Test JSON unmarshalling with actual mock response file
func Test_UnmarshalMockResponse(t *testing.T) {
	// Arrange
	mockResponseBytes, err := os.ReadFile("../../mockResponses/response.json")
	require.NoError(t, err, "Failed to read mock response file")

	// Act
	var response LinearViewerResponse
	err = json.Unmarshal(mockResponseBytes, &response)

	// Assert
	require.NoError(t, err, "Failed to unmarshal mock response")
	assert.NotNil(t, response.Data.Viewer.AssignedIssues.Edges)
	assert.Equal(t, 2, len(response.Data.Viewer.AssignedIssues.Edges))

	// Verify first issue
	firstIssue := response.Data.Viewer.AssignedIssues.Edges[0].Node
	assert.Equal(t, "test-issue-id-001", firstIssue.ID)
	assert.Equal(t, "Implement user authentication feature", firstIssue.Title)
	assert.Contains(t, firstIssue.URL, "linear.app")
}

// Test GetIssueDetails with missing issueID
func Test_GetIssueDetails_MissingIssueID(t *testing.T) {
	// Arrange
	config := createTestConfig("https://api.linear.app/graphql", "test-token")

	// Act
	_, err := GetIssueDetails(&http.Client{}, "", config)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "issueID is required")
}

// Test GetIssueDetails with successful response
func Test_GetIssueDetails_Success(t *testing.T) {
	// Arrange - Load mock issue detail response
	mockResponseBytes, err := os.ReadFile("../../mockResponses/issue-detail.json")
	require.NoError(t, err, "Failed to read mock issue detail file")

	// Create mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-token", r.Header.Get("Authorization"))

		// Verify GraphQL query structure
		var requestBody LinearViewerRequest
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		require.NoError(t, err)
		assert.Equal(t, "GetIssueDetails", requestBody.OperationName)
		assert.Contains(t, requestBody.Query, "issue(id:")
		assert.Contains(t, requestBody.Query, "description")
		assert.Contains(t, requestBody.Query, "labels")
		assert.Contains(t, requestBody.Query, "comments")

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(mockResponseBytes)
	}))
	defer mockServer.Close()

	// Setup config with mock server URL
	config := createTestConfig(mockServer.URL, "test-api-token")

	// Act
	issueID := "test-issue-id-001"
	result, err := GetIssueDetails(&http.Client{}, issueID, config)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, issueID, result.ID)
	assert.Equal(t, "Implement user authentication feature", result.Title)
	assert.Equal(t, "TEST-123", result.Identifier)
	assert.Equal(t, "In Progress", result.State.Name)
	assert.Equal(t, "High", result.PriorityLabel)
	assert.NotEmpty(t, result.Description)
	assert.Contains(t, result.Description, "Overview")

	// Verify labels
	assert.Equal(t, 2, len(result.Labels.Nodes))
	assert.Equal(t, "backend", result.Labels.Nodes[0].Name)
	assert.Equal(t, "security", result.Labels.Nodes[1].Name)

	// Verify comments
	assert.Equal(t, 2, len(result.Comments.Nodes))
	assert.Equal(t, "Alice Developer", result.Comments.Nodes[0].User.Name)
	assert.Contains(t, result.Comments.Nodes[0].Body, "Started implementation")

	// Verify assignee
	assert.Equal(t, "Alice Developer", result.Assignee.Name)
	assert.Equal(t, "alice@example.com", result.Assignee.Email)
}

// Test GetIssueDetails with API error
func Test_GetIssueDetails_APIError(t *testing.T) {
	// Arrange - Create mock server returning error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorResponse := map[string]interface{}{
			"errors": []map[string]interface{}{
				{
					"message": "Issue not found",
					"extensions": map[string]interface{}{
						"code": "NOT_FOUND",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(errorResponse)
	}))
	defer mockServer.Close()

	config := createTestConfig(mockServer.URL, "test-token")

	// Act
	result, err := GetIssueDetails(&http.Client{}, "invalid-id", config)

	// Assert
	require.NoError(t, err) // HTTP request succeeds even if API returns error
	// Result will have empty fields because error response doesn't match our struct
	assert.Equal(t, "", result.ID)
	assert.Equal(t, "", result.Title)
}
