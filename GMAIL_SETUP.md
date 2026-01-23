# Gmail API Setup Instructions

## Issue: Access blocked: Error 403

You're seeing this error because your OAuth app needs to be configured for local development. Here's how to fix it:

## Step 1: Update OAuth Redirect URI

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Find your OAuth 2.0 Client ID (the one you downloaded as `mastercrab_google_credentials_desktop.json`)
3. Click on it to edit
4. Under **Authorized redirect URIs**, add:
   ```
   http://localhost:8080/callback
   ```
5. Click **SAVE**

## Step 2: Add Test Users (for unverified apps)

Since your app isn't verified by Google yet, you need to add yourself as a test user:

1. Go to [OAuth consent screen](https://console.cloud.google.com/apis/credentials/consent)
2. Scroll down to **Test users**
3. Click **ADD USERS**
4. Enter your Gmail address
5. Click **SAVE**

## Step 3: Verify API is Enabled

1. Go to [APIs & Services > Library](https://console.cloud.google.com/apis/library)
2. Search for "Gmail API"
3. Make sure it's **ENABLED**

## Step 4: Rebuild and Test

```bash
go build -o mastercrab
./mastercrab gmail analyze --analysis-type sender-stats
```

The browser should now open to `http://localhost:8080/callback` and authentication should complete automatically.

## Troubleshooting

### "Access blocked: mastercrab has not completed the Google verification process"

**Solution**: Add yourself as a test user (Step 2 above). Your app doesn't need to be verified for personal use.

### "Redirect URI mismatch"

**Solution**: Make sure you added `http://localhost:8080/callback` exactly as shown in Step 1.

### Browser doesn't open

**Solution**: Copy the URL from the terminal and paste it into your browser manually.

### Port 8080 already in use

**Solution**: Stop any other service using port 8080, or modify the port in `pkg/gmail/auth.go` (line 100 and 93).

## Notes

- **Testing Mode**: Your app can stay in testing mode indefinitely for personal use. You only need verification if you want to distribute it to other users.
- **Scopes**: The app requests `gmail.readonly` (for analysis) and `gmail.modify` (for deletion). These are appropriate for the functionality.
- **Token Caching**: After first successful auth, the token is cached in `~/.crab/gmail-token.json` and you won't need to authenticate again.
