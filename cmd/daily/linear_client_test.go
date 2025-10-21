package daily

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

var handler = func(data []byte) http.HandlerFunc {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(data)
	})
}

func helper_LinearViewerRequestBodyExtractor(t *testing.T, request *http.Request) LinearViewerRequest {
	var requestBody = make([]byte, request.ContentLength)
	bytesRead, readError := request.Body.Read(requestBody)
	if bytesRead != int(request.ContentLength) {
		t.Errorf("Failed to read full request body content: %s", readError.Error())
	}

	var unmarshalledBody LinearViewerRequest
	error := json.Unmarshal(requestBody, &unmarshalledBody)
	if error != nil {
		t.Errorf("Failure unmarshalling request body as JSON: %s", error.Error())
	}
	return unmarshalledBody
}

// func TestServer(t *testing.T) {

// 	linearResponse, err := json.Marshal(map[string]string{
// 		"hello": "world",
// 	})

// 	if err != nil {
// 		t.Errorf("Failed to create mock linear response %s\n", err)
// 	}

// 	linearTestServer := httptest.NewServer(handler(linearResponse))
// 	defer linearTestServer.Close()

// 	got :=

// 	if got := Hello(); got != want {
// 		t.Errorf("Hello() = %q, want %q", got, want)
// 	}

// }

func Test_BuildLinearViewerRequest_IsPOST(t *testing.T) {
	expectedMethod := "POST"

	actualRequest := buildViewerAssignedIssuesRequest("")

	assert.Equal(t, expectedMethod, actualRequest.Method)
}

func Test_BuildLinearViewerRequest_HasOperationName(t *testing.T) {
	// Arrange
	expectedOperationName := "MyAssignedIssues"

	// Act
	generatedRequest := buildViewerAssignedIssuesRequest("")
	generatedRequestBody := helper_LinearViewerRequestBodyExtractor(t, generatedRequest)

	// Assert
	assert.Equal(t, expectedOperationName, generatedRequestBody.OperationName)
	assert.Contains(t, generatedRequestBody.Query, fmt.Sprintf("query %s", expectedOperationName))
}

func Test_BuildLinearViewerRequest_HasDefaultDateFilter(t *testing.T) {
	// Arrange
	expectedDefaultDateFilter := "assignedIssues(filter: { updatedAt: { gte: \"-P1D\" }})"

	// Act
	generatedRequest := buildViewerAssignedIssuesRequest("")
	generatedRequestBody := helper_LinearViewerRequestBodyExtractor(t, generatedRequest)

	// Assert
	assert.Contains(t, generatedRequestBody.Query, expectedDefaultDateFilter)
}

func Test_BuildLinearViewerRequest_UserDefinedDateFilter(t *testing.T) {
	// Arrange
	var dateFilter = "-P5D"
	expectedDefaultDateFilter := fmt.Sprintf("assignedIssues(filter: { updatedAt: { gte: \"%s\" }})", dateFilter)

	// Act
	generatedRequest := buildViewerAssignedIssuesRequest(dateFilter)
	generatedRequestBody := helper_LinearViewerRequestBodyExtractor(t, generatedRequest)

	// Assert
	assert.Contains(t, generatedRequestBody.Query, expectedDefaultDateFilter)
}

func Test_BuildLinearViewerRequest_HasAuthorization(t *testing.T) {
	// Arrange
	expectedAuthHeader := "Bearer: abdc120938"

	// Act
	generatedRequest := buildViewerAssignedIssuesRequest("")

	// Assert
	assert.Equal(t, expectedAuthHeader, generatedRequest.Header["Authorization"][0])
}
