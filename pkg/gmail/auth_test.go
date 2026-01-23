package gmail

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
)

func createTestConfig(credFile, tokenFile string) *viper.Viper {
	v := viper.New()
	v.Set("gmail.credentialsFile", credFile)
	v.Set("gmail.tokenFile", tokenFile)
	v.Set("gmail.scopes", []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.modify",
	})
	return v
}

func TestNewAuthManager(t *testing.T) {
	config := createTestConfig("~/.crab/credentials.json", "~/.crab/token.json")
	am := NewAuthManager(config)

	assert.NotNil(t, am)
	assert.Equal(t, config, am.config)
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "home directory expansion",
			input:    "~/test/path",
			expected: filepath.Join(os.Getenv("HOME"), "test/path"),
		},
		{
			name:     "no expansion needed",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "relative path",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSaveToken(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "test-token.json")

	// Create test token
	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
	}

	// Create auth manager
	config := createTestConfig("", tokenFile)
	am := NewAuthManager(config)

	// Save token
	err := am.saveToken(tokenFile, token)
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(tokenFile)
	assert.NoError(t, err)

	// Verify file permissions (should be 0600)
	info, err := os.Stat(tokenFile)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestTokenFromFile(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "test-token.json")

	// Create test token and save it
	expectedToken := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
	}

	config := createTestConfig("", tokenFile)
	am := NewAuthManager(config)
	err := am.saveToken(tokenFile, expectedToken)
	assert.NoError(t, err)

	// Read token back
	token, err := am.tokenFromFile(tokenFile)
	assert.NoError(t, err)
	assert.Equal(t, expectedToken.AccessToken, token.AccessToken)
	assert.Equal(t, expectedToken.RefreshToken, token.RefreshToken)
	assert.Equal(t, expectedToken.TokenType, token.TokenType)
}

func TestTokenFromFile_NotFound(t *testing.T) {
	config := createTestConfig("", "/nonexistent/token.json")
	am := NewAuthManager(config)

	token, err := am.tokenFromFile("/nonexistent/token.json")
	assert.Error(t, err)
	assert.Nil(t, token)
}

func TestGetAuthClient_MissingCredentialsFile(t *testing.T) {
	// Create config without credentials file
	config := viper.New()
	am := NewAuthManager(config)

	client, err := am.GetAuthClient(nil)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "gmail.credentialsFile not configured")
}
