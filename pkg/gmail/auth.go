package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

// AuthManager handles OAuth2 authentication for Gmail API
type AuthManager struct {
	config *viper.Viper
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config *viper.Viper) *AuthManager {
	return &AuthManager{
		config: config,
	}
}

// GetAuthClient returns an authenticated HTTP client for Gmail API
// It handles the OAuth2 flow: reads cached token or opens browser for first auth
func (am *AuthManager) GetAuthClient(ctx context.Context) (*http.Client, error) {
	// Get credentials file path from config
	credentialsFile := am.config.GetString("gmail.credentialsFile")
	if credentialsFile == "" {
		return nil, fmt.Errorf("gmail.credentialsFile not configured in crab.yaml")
	}

	// Expand home directory if needed
	credentialsFile = expandPath(credentialsFile)

	// Read credentials file
	credBytes, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file %s: %w", credentialsFile, err)
	}

	// Get scopes from config
	scopes := am.config.GetStringSlice("gmail.scopes")
	if len(scopes) == 0 {
		// Default scopes if not configured
		scopes = []string{
			gmail.GmailReadonlyScope,
			gmail.GmailModifyScope,
		}
	}

	// Parse credentials
	oauthConfig, err := google.ConfigFromJSON(credBytes, scopes...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials file: %w", err)
	}

	// Get token file path
	tokenFile := am.config.GetString("gmail.tokenFile")
	if tokenFile == "" {
		tokenFile = "~/.crab/gmail-token.json"
	}
	tokenFile = expandPath(tokenFile)

	// Try to load cached token
	token, err := am.tokenFromFile(tokenFile)
	if err != nil {
		// No cached token, get new one from web
		token, err = am.getTokenFromWeb(ctx, oauthConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to get token from web: %w", err)
		}
		// Save token for future use
		if err := am.saveToken(tokenFile, token); err != nil {
			return nil, fmt.Errorf("unable to save token: %w", err)
		}
	}

	return oauthConfig.Client(ctx, token), nil
}

// getTokenFromWeb requests a token from the web, then opens browser for authorization
// Uses a local HTTP server to receive the OAuth callback
func (am *AuthManager) getTokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	// Set redirect URI to localhost
	config.RedirectURL = "http://localhost:8080/callback"

	// Create a channel to receive the authorization code
	codeCh := make(chan string)
	errCh := make(chan error)

	// Start local HTTP server to receive OAuth callback
	server := &http.Server{Addr: ":8080"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code received")
			http.Error(w, "Authorization failed: no code received", http.StatusBadRequest)
			return
		}

		// Send success response to browser
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<html>
			<head><title>Authentication Successful</title></head>
			<body style="font-family: sans-serif; text-align: center; padding: 50px;">
				<h1>✅ Authentication Successful!</h1>
				<p>You can close this window and return to the terminal.</p>
			</body>
			</html>
		`)

		codeCh <- code
	})

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("failed to start local server: %w", err)
		}
	}()

	// Ensure server is shut down when done
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	// Generate auth URL
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("\n🔐 Gmail Authentication Required\n")
	fmt.Printf("Opening browser for authorization...\n")
	fmt.Printf("If browser doesn't open, visit this URL:\n%s\n\n", authURL)
	fmt.Printf("Waiting for authorization...\n")

	// Open browser
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("⚠️  Failed to open browser automatically: %s\n", err)
	}

	// Wait for code or error
	var authCode string
	select {
	case authCode = <-codeCh:
		// Got the code
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authentication timeout after 5 minutes")
	}

	// Exchange code for token
	token, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %w", err)
	}

	return token, nil
}

// tokenFromFile retrieves a token from a local file
func (am *AuthManager) tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

// saveToken saves a token to a file path
func (am *AuthManager) saveToken(path string, token *oauth2.Token) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("unable to create token directory: %w", err)
	}

	// Create file with restricted permissions (0600)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to create token file: %w", err)
	}
	defer f.Close()

	// Write token
	if err := json.NewEncoder(f).Encode(token); err != nil {
		return fmt.Errorf("unable to encode token: %w", err)
	}

	fmt.Printf("✅ Token saved to %s\n", path)
	return nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(home, path[1:])
}

// openBrowser opens the default browser with the given URL
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("unsupported platform")
	}

	return exec.Command(cmd, args...).Start()
}
