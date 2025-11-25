# Using Google OAuth Tokens Directly (No Custom JWT)

## Overview

Instead of creating custom JWT tokens, we'll use Google's OAuth tokens directly:
- **Google Access Token** - Short-lived (1 hour), use for authenticating requests
- **Google Refresh Token** - Long-lived, use to get new access tokens
- Store both in HTTP-only cookies
- Validate by checking with Google's API

## Benefits

✅ No need to create/sign custom JWTs  
✅ Google manages token security  
✅ Google handles token expiration  
✅ Simpler implementation  
✅ Can validate tokens with Google's API  

## Implementation

### 1. Backend Changes (Go)

#### A. Update OAuth Configuration

```go
func initOAuth() {
    conf = &oauth2.Config{
        ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
        ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
        RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
        Scopes: []string{
            "https://www.googleapis.com/auth/userinfo.email",
            "https://www.googleapis.com/auth/userinfo.profile",
        },
        Endpoint: google.Endpoint,
    }
}

func redirectToGoogleLogin(w http.ResponseWriter, r *http.Request) {
    // ... existing state generation ...
    
    // IMPORTANT: Add this to get refresh token
    url := conf.AuthCodeURL(state, 
        oauth2.AccessTypeOffline,  // Request refresh token
        oauth2.ApprovalForce,      // Force prompt to ensure we get refresh token
    )
    
    http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}
```

#### B. Update Google OAuth Callback

Replace your current `handleGoogleCallback` function:

```go
func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
    state := r.URL.Query().Get("state")
    
    // Validate state
    if _, ok := oauthStateCache[state]; !ok {
        log.Printf("Invalid state: %s", state)
        http.Redirect(w, r, fmt.Sprintf("%s/?error=invalid_state", frontendURL), http.StatusTemporaryRedirect)
        return
    }
    delete(oauthStateCache, state)
    
    code := r.URL.Query().Get("code")
    
    // Exchange code for token (includes access_token and refresh_token)
    token, err := conf.Exchange(context.Background(), code)
    if err != nil {
        log.Printf("Error exchanging code for token: %v", err)
        http.Redirect(w, r, fmt.Sprintf("%s/?error=token_exchange_failed", frontendURL), http.StatusTemporaryRedirect)
        return
    }
    
    // Get user info using access token
    response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
    if err != nil {
        log.Printf("Error getting user info: %v", err)
        http.Redirect(w, r, fmt.Sprintf("%s/?error=userinfo_failed", frontendURL), http.StatusTemporaryRedirect)
        return
    }
    defer response.Body.Close()
    
    data, err := io.ReadAll(response.Body)
    if err != nil {
        log.Printf("Error reading response body: %v", err)
        return
    }
    
    var oAuthData OAuthData
    err = json.Unmarshal(data, &oAuthData)
    if err != nil {
        log.Printf("Error unmarshalling user info: %v", err)
        return
    }
    
    // Check email verification
    if !oAuthData.Verified_email {
        log.Printf("Email not verified for user: %s", oAuthData.Email)
        http.Redirect(w, r, fmt.Sprintf("%s/?error=verification_failed", frontendURL), http.StatusTemporaryRedirect)
        return
    }
    
    // Log success
    log.Printf("OAuth successful for user: %s", oAuthData.Email)
    
    // Set Google Access Token cookie (1 hour expiry)
    http.SetCookie(w, &http.Cookie{
        Name:     "google_access_token",
        Value:    token.AccessToken,
        Path:     "/",
        MaxAge:   3600, // 1 hour (Google's default)
        HttpOnly: true,
        Secure:   false, // Set to true in production with HTTPS
        SameSite: http.SameSiteLaxMode,
    })
    
    // Set Google Refresh Token cookie (doesn't expire)
    if token.RefreshToken != "" {
        http.SetCookie(w, &http.Cookie{
            Name:     "google_refresh_token",
            Value:    token.RefreshToken,
            Path:     "/",
            MaxAge:   0, // Session cookie (or set to 30 days: 2592000)
            HttpOnly: true,
            Secure:   false, // Set to true in production
            SameSite: http.SameSiteLaxMode,
        })
        log.Printf("Refresh token set for user: %s", oAuthData.Email)
    } else {
        log.Printf("Warning: No refresh token received for user: %s", oAuthData.Email)
    }
    
    // Store user email for later validation (optional)
    http.SetCookie(w, &http.Cookie{
        Name:     "user_email",
        Value:    oAuthData.Email,
        Path:     "/",
        MaxAge:   3600,
        HttpOnly: false, // Frontend needs to read this
        Secure:   false,
        SameSite: http.SameSiteLaxMode,
    })
    
    // Redirect to frontend
    frontendURL := os.Getenv("FRONTEND_URL")
    if frontendURL == "" {
        frontendURL = "http://localhost:3000"
    }
    
    http.Redirect(w, r, fmt.Sprintf("%s/?login=success&email=%s", frontendURL, oAuthData.Email), http.StatusTemporaryRedirect)
}
```

#### C. Add Authentication Middleware

```go
// Middleware to validate Google access token
func googleAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Get access token from cookie
        cookie, err := r.Cookie("google_access_token")
        if err != nil {
            http.Error(w, `{"error": "Unauthorized - No access token"}`, http.StatusUnauthorized)
            return
        }
        
        // Validate token with Google
        userInfo, err := validateGoogleToken(cookie.Value)
        if err != nil {
            // Token expired or invalid - try to refresh
            log.Printf("Access token invalid: %v", err)
            http.Error(w, `{"error": "Unauthorized - Invalid token"}`, http.StatusUnauthorized)
            return
        }
        
        // Add user info to context
        ctx := context.WithValue(r.Context(), "user_email", userInfo.Email)
        ctx = context.WithValue(ctx, "user_id", userInfo.Id)
        
        // Call next handler
        next.ServeHTTP(w, r.WithContext(ctx))
    }
}

// Validate Google access token
func validateGoogleToken(accessToken string) (*OAuthData, error) {
    // Call Google's tokeninfo endpoint
    response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    
    if response.StatusCode != 200 {
        return nil, fmt.Errorf("invalid token, status: %d", response.StatusCode)
    }
    
    data, err := io.ReadAll(response.Body)
    if err != nil {
        return nil, err
    }
    
    var userInfo OAuthData
    err = json.Unmarshal(data, &userInfo)
    if err != nil {
        return nil, err
    }
    
    return &userInfo, nil
}
```

#### D. Add Token Refresh Endpoint

```go
func handleRefreshToken(w http.ResponseWriter, r *http.Request) {
    // Get refresh token from cookie
    refreshCookie, err := r.Cookie("google_refresh_token")
    if err != nil {
        http.Error(w, `{"error": "No refresh token"}`, http.StatusUnauthorized)
        return
    }
    
    // Use Google's OAuth2 library to refresh the token
    token := &oauth2.Token{
        RefreshToken: refreshCookie.Value,
    }
    
    // Get new token from Google
    tokenSource := conf.TokenSource(context.Background(), token)
    newToken, err := tokenSource.Token()
    if err != nil {
        log.Printf("Error refreshing token: %v", err)
        http.Error(w, `{"error": "Failed to refresh token"}`, http.StatusUnauthorized)
        return
    }
    
    // Set new access token cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "google_access_token",
        Value:    newToken.AccessToken,
        Path:     "/",
        MaxAge:   3600,
        HttpOnly: true,
        Secure:   false,
        SameSite: http.SameSiteLaxMode,
    })
    
    log.Printf("Token refreshed successfully")
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "message": "Token refreshed successfully",
    })
}
```

#### E. Add Logout Endpoint

```go
func handleLogout(w http.ResponseWriter, r *http.Request) {
    // Get access token to revoke it with Google (optional but recommended)
    accessCookie, _ := r.Cookie("google_access_token")
    
    if accessCookie != nil {
        // Revoke token with Google
        revokeURL := fmt.Sprintf("https://oauth2.googleapis.com/revoke?token=%s", accessCookie.Value)
        resp, err := http.Post(revokeURL, "application/x-www-form-urlencoded", nil)
        if err == nil {
            resp.Body.Close()
            log.Printf("Token revoked with Google")
        }
    }
    
    // Clear all auth cookies
    http.SetCookie(w, &http.Cookie{
        Name:     "google_access_token",
        Value:    "",
        Path:     "/",
        MaxAge:   -1,
        HttpOnly: true,
    })
    
    http.SetCookie(w, &http.Cookie{
        Name:     "google_refresh_token",
        Value:    "",
        Path:     "/",
        MaxAge:   -1,
        HttpOnly: true,
    })
    
    http.SetCookie(w, &http.Cookie{
        Name:     "user_email",
        Value:    "",
        Path:     "/",
        MaxAge:   -1,
    })
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "message": "Logged out successfully",
    })
}
```

#### F. Protect API Routes

```go
func main() {
    // ... existing setup ...
    
    // PUBLIC routes (no auth required)
    router.HandleFunc("/api/go/auth/google", redirectToGoogleLogin).Methods("GET")
    router.HandleFunc("/api/go/auth/google/callback", handleGoogleCallback).Methods("GET")
    router.HandleFunc("/api/go/auth/refresh", handleRefreshToken).Methods("POST")
    router.HandleFunc("/api/go/auth/logout", handleLogout).Methods("POST")
    
    // PROTECTED routes (require Google access token)
    router.HandleFunc("/api/go/users", googleAuthMiddleware(getUsers(db))).Methods("GET")
    router.HandleFunc("/api/go/users", googleAuthMiddleware(createUser(db))).Methods("POST")
    router.HandleFunc("/api/go/users/{id}", googleAuthMiddleware(getUser(db))).Methods("GET")
    router.HandleFunc("/api/go/users/{id}", googleAuthMiddleware(updateUser(db))).Methods("PUT")
    router.HandleFunc("/api/go/users/{id}", googleAuthMiddleware(deleteUser(db))).Methods("DELETE")
    
    // ... rest of setup ...
}
```

#### G. Fix CORS for Cookies

```go
func enableCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Get frontend URL
        frontendURL := os.Getenv("FRONTEND_URL")
        if frontendURL == "" {
            frontendURL = "http://localhost:3000"
        }
        
        // CRITICAL: Must use specific origin, not "*" for cookies
        w.Header().Set("Access-Control-Allow-Origin", frontendURL)
        w.Header().Set("Access-Control-Allow-Credentials", "true") // REQUIRED for cookies
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### 2. Frontend Changes (React/Next.js)

#### A. Update API Calls to Include Credentials

Update `UserInterface.tsx`:

```typescript
const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000';

// Wrapper function for API calls with auto-refresh
const apiCall = async (url: string, options: RequestInit = {}): Promise<Response> => {
  const response = await fetch(url, {
    ...options,
    credentials: 'include', // CRITICAL: Send cookies with request
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  });
  
  // If unauthorized (token expired), try to refresh
  if (response.status === 401) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      // Retry the original request
      return fetch(url, {
        ...options,
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          ...options.headers,
        },
      });
    } else {
      // Refresh failed, redirect to login
      window.location.href = '/';
      throw new Error('Session expired');
    }
  }
  
  return response;
};

// Function to refresh access token
const refreshAccessToken = async (): Promise<boolean> => {
  try {
    const response = await fetch(`${apiUrl}/api/go/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    return response.ok;
  } catch (error) {
    console.error('Token refresh failed:', error);
    return false;
  }
};

// Update all API calls to use apiCall wrapper
const fetchData = async () => {
  try {
    const response = await apiCall(`${apiUrl}/api/${backendName}/users`);
    const data = await response.json();
    setUsers(data.reverse());
  } catch (error) {
    console.error('Error fetching data:', error);
  }
};

const createUser = async (e: React.FormEvent<HTMLFormElement>) => {
  e.preventDefault();
  try {
    const response = await apiCall(`${apiUrl}/api/${backendName}/users`, {
      method: 'POST',
      body: JSON.stringify(newUser),
    });
    const data = await response.json();
    setUsers([data, ...users]);
    setNewUser({ name: '', email: '', description: '' });
  } catch (error) {
    console.error('Error creating user:', error);
  }
};

// ... update other functions similarly ...
```

#### B. Update Authentication Check

Update `dashboard.tsx` and `index.tsx`:

```typescript
// Check authentication by trying to fetch user info
const checkAuth = async (): Promise<string | null> => {
  try {
    // Try to get user email from cookie or validate with backend
    const response = await fetch(`${apiUrl}/api/go/users`, {
      credentials: 'include',
    });
    
    if (response.ok) {
      // Get email from cookie
      const email = document.cookie
        .split('; ')
        .find(row => row.startsWith('user_email='))
        ?.split('=')[1];
      
      return email || 'user@example.com';
    }
    
    // Try to refresh token
    const refreshResponse = await fetch(`${apiUrl}/api/go/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    
    if (refreshResponse.ok) {
      const email = document.cookie
        .split('; ')
        .find(row => row.startsWith('user_email='))
        ?.split('=')[1];
      return email || null;
    }
    
    return null;
  } catch (error) {
    console.error('Auth check failed:', error);
    return null;
  }
};
```

#### C. Update Logout Function

```typescript
const handleLogout = async () => {
  try {
    await fetch(`${apiUrl}/api/go/auth/logout`, {
      method: 'POST',
      credentials: 'include',
    });
  } catch (error) {
    console.error('Logout error:', error);
  } finally {
    // Redirect to home page
    router.replace('/');
  }
};
```

#### D. Remove sessionStorage Usage

Delete all lines with:
- `sessionStorage.setItem(...)`
- `sessionStorage.getItem(...)`
- `sessionStorage.removeItem(...)`

Tokens are now in HTTP-only cookies managed by the browser automatically.

### 3. Environment Variables

Update `.env`:

```env
# Backend
GOOGLE_CLIENT_ID=your_client_id
GOOGLE_CLIENT_SECRET=your_client_secret
GOOGLE_REDIRECT_URL=http://localhost:8000/api/go/auth/google/callback
FRONTEND_URL=http://localhost:3000
DATABASE_URL=postgresql://user:password@db:5432/dbname

# Frontend
NEXT_PUBLIC_API_URL=http://localhost:8000
```

## How It Works

### Token Flow:

1. **Login:**
   - User clicks "Continue with Google"
   - Google authenticates and returns `access_token` + `refresh_token`
   - Backend stores both in HTTP-only cookies
   - Redirects to `/dashboard`

2. **API Requests:**
   - Browser automatically sends cookies with every request
   - Backend validates `google_access_token` with Google API
   - If valid → process request
   - If invalid → return 401

3. **Token Refresh:**
   - Frontend detects 401 error
   - Calls `/api/go/auth/refresh`
   - Backend uses `google_refresh_token` to get new `access_token` from Google
   - Sets new `access_token` cookie
   - Frontend retries original request

4. **Logout:**
   - Call `/api/go/auth/logout`
   - Backend revokes token with Google
   - Clears all cookies
   - Redirects to home

## Security Benefits

✅ **HTTP-only cookies** - JavaScript can't access tokens (XSS protection)  
✅ **Google validates tokens** - Don't need to maintain token validation logic  
✅ **Automatic refresh** - Seamless user experience  
✅ **Google token revocation** - Proper logout  
✅ **No token storage** - Tokens never in localStorage/sessionStorage  

## Important Notes

1. **Refresh Token:**
   - Google only provides refresh token on FIRST login
   - If you revoke access in Google account, user must re-authenticate
   - Use `oauth2.ApprovalForce` to always get refresh token

2. **Token Expiry:**
   - Google access tokens expire after 1 hour
   - Implement auto-refresh in frontend
   - Refresh tokens don't expire (until revoked)

3. **Production:**
   - Set `Secure: true` on cookies (requires HTTPS)
   - Use environment-specific FRONTEND_URL
   - Consider storing refresh tokens in database for revocation

## Testing

1. Start backend: `cd backend_go && go run main.go`
2. Start frontend: `cd frontend_react && npm run dev`
3. Visit `http://localhost:3000`
4. Login with Google
5. Check cookies in browser DevTools (Application → Cookies)
6. Make API requests → cookies sent automatically
7. Wait 1 hour or manually delete access_token cookie → should auto-refresh

That's it! Much simpler than custom JWT. 🎉



