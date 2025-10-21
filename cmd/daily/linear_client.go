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

func buildViewerAssignedIssuesRequest(date_filter string, auth_token string) *http.Request {
	// If date_filter is empty, default to the last 24 hours
	if date_filter == "" {
		date_filter = "-P1D"
	}

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
	jsonValue, json_marshall_error := json.Marshal(linearRequest)
	if json_marshall_error != nil {
		fmt.Printf("Failed to create JSON payload for graphql request %s\n", json_marshall_error)
	}

	request, request_creation_error := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(jsonValue))
	request.Header.Set("Content-Type", "application/json")

	// TODO: Re-enable authorization
	request.Header.Set("Authorization", auth_token)

	if request_creation_error != nil {
		fmt.Printf("Failed to create request %s\n", request_creation_error)
	}
	return request
}

func GetViewerAssignedIssues(client *http.Client, date_filter string, config *viper.Viper) (LinearViewer, error) {
	linearAuth := config.GetString("linear.apiToken")
	request := buildViewerAssignedIssuesRequest(date_filter, linearAuth)
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
	// assignedIssuesJSON, err := json.MarshalIndent(assignedIssues, "", "  ")

	// if err != nil {
	// 	fmt.Printf("Error marshalling assignedIssues to JSON: %s\n", err)
	// } else {
	// 	fmt.Printf("Response from Linear:\n%s\n", assignedIssuesJSON)
	// }
	return viewer.Data.LinearViewer, nil
}
