# Proper Authentication Implementation Guide

## Current Implementation Problems

### What's Wrong:
1. ❌ Refresh token cookie is set but never validated or used
2. ❌ No JWT access tokens generated
3. ❌ Frontend uses sessionStorage (insecure, can be accessed by JavaScript)
4. ❌ No token refresh mechanism
5. ❌ Backend API routes are unprotected (anyone can access them)
6. ❌ CORS set to `*` which blocks credentials/cookies
7. ❌ No session management or token expiration

### Security Risks:
- Anyone can call your API endpoints without authentication
- XSS attacks can steal tokens from sessionStorage
- No way to revoke access
- No proper logout mechanism
- Google's refresh token is stored but never refreshed

## What You Need to Implement

### 1. Backend Changes (Go)

#### A. Install JWT Library
```bash
go get github.com/golang-jwt/jwt/v5
```

#### B. Generate JWT Access & Refresh Tokens
After successful OAuth, generate TWO tokens:

**Access Token:**
- Short-lived (15 minutes)
- Sent in HTTP-only cookie
- Used for API requests
- Contains user info (email, ID)

**Refresh Token:**
- Long-lived (7 days)
- Sent in HTTP-only cookie
- Used to get new access tokens
- Stored in database with user session

#### C. Backend Structure:

```go
// Add to main.go

import (
    "github.com/golang-jwt/jwt/v5"
    "time"
)

// JWT Claims structure
type Claims struct {
    Email string `json:"email"`
    UserID int   `json:"user_id"`
    jwt.RegisteredClaims
}

var jwtSecret = []byte(os.Getenv("JWT_SECRET")) // Set in .env

// Generate Access Token (15 min expiry)
func generateAccessToken(email string, userID int) (string, error) {
    claims := &Claims{
        Email: email,
        UserID: userID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(jwtSecret)
}

// Generate Refresh Token (7 days expiry)
func generateRefreshToken(email string, userID int) (string, error) {
    claims := &Claims{
        Email: email,
        UserID: userID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(jwtSecret)
}

// Validate JWT Token
func validateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return jwtSecret, nil
    })
    
    if err != nil {
        return nil, err
    }
    
    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }
    
    return nil, fmt.Errorf("invalid token")
}
```

#### D. Update OAuth Callback:

```go
func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
    // ... existing OAuth code ...
    
    // After successful OAuth verification:
    if oAuthData.Verified_email {
        // Get or create user in database
        userID := getOrCreateUser(db, oAuthData.Email, oAuthData.Id)
        
        // Generate tokens
        accessToken, _ := generateAccessToken(oAuthData.Email, userID)
        refreshToken, _ := generateRefreshToken(oAuthData.Email, userID)
        
        // Store refresh token in database (for revocation)
        storeRefreshToken(db, userID, refreshToken)
        
        // Set HTTP-only cookies
        http.SetCookie(w, &http.Cookie{
            Name:     "access_token",
            Value:    accessToken,
            Path:     "/",
            MaxAge:   900, // 15 minutes
            HttpOnly: true,
            Secure:   true, // HTTPS only in production
            SameSite: http.SameSiteLaxMode,
        })
        
        http.SetCookie(w, &http.Cookie{
            Name:     "refresh_token",
            Value:    refreshToken,
            Path:     "/",
            MaxAge:   604800, // 7 days
            HttpOnly: true,
            Secure:   true,
            SameSite: http.SameSiteLaxMode,
        })
        
        // Redirect to frontend
        http.Redirect(w, r, frontendURL + "/?login=success", http.StatusTemporaryRedirect)
    }
}
```

#### E. Add Authentication Middleware:

```go
// Middleware to protect routes
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Get access token from cookie
        cookie, err := r.Cookie("access_token")
        if err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        
        // Validate token
        claims, err := validateToken(cookie.Value)
        if err != nil {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        
        // Add user info to context
        ctx := context.WithValue(r.Context(), "email", claims.Email)
        ctx = context.WithValue(ctx, "user_id", claims.UserID)
        
        // Call next handler
        next.ServeHTTP(w, r.WithContext(ctx))
    }
}
```

#### F. Protect API Routes:

```go
func main() {
    // ... existing code ...
    
    // Protected routes - require authentication
    router.HandleFunc("/api/go/users", authMiddleware(getUsers(db))).Methods("GET")
    router.HandleFunc("/api/go/users", authMiddleware(createUser(db))).Methods("POST")
    router.HandleFunc("/api/go/users/{id}", authMiddleware(updateUser(db))).Methods("PUT")
    router.HandleFunc("/api/go/users/{id}", authMiddleware(deleteUser(db))).Methods("DELETE")
    
    // Token refresh endpoint
    router.HandleFunc("/api/go/auth/refresh", handleRefreshToken(db)).Methods("POST")
    
    // Logout endpoint
    router.HandleFunc("/api/go/auth/logout", handleLogout(db)).Methods("POST")
}
```

#### G. Add Token Refresh Endpoint:

```go
func handleRefreshToken(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Get refresh token from cookie
        cookie, err := r.Cookie("refresh_token")
        if err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        
        // Validate refresh token
        claims, err := validateToken(cookie.Value)
        if err != nil {
            http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
            return
        }
        
        // Check if refresh token exists in database
        if !isRefreshTokenValid(db, claims.UserID, cookie.Value) {
            http.Error(w, "Token revoked", http.StatusUnauthorized)
            return
        }
        
        // Generate new access token
        newAccessToken, _ := generateAccessToken(claims.Email, claims.UserID)
        
        // Set new access token cookie
        http.SetCookie(w, &http.Cookie{
            Name:     "access_token",
            Value:    newAccessToken,
            Path:     "/",
            MaxAge:   900,
            HttpOnly: true,
            Secure:   true,
            SameSite: http.SameSiteLaxMode,
        })
        
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{"message": "Token refreshed"})
    }
}
```

#### H. Add Logout Endpoint:

```go
func handleLogout(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Get refresh token
        cookie, _ := r.Cookie("refresh_token")
        
        if cookie != nil {
            // Revoke refresh token in database
            revokeRefreshToken(db, cookie.Value)
        }
        
        // Clear cookies
        http.SetCookie(w, &http.Cookie{
            Name:     "access_token",
            Value:    "",
            Path:     "/",
            MaxAge:   -1,
            HttpOnly: true,
        })
        
        http.SetCookie(w, &http.Cookie{
            Name:     "refresh_token",
            Value:    "",
            Path:     "/",
            MaxAge:   -1,
            HttpOnly: true,
        })
        
        w.WriteHeader(http.StatusOK)
    }
}
```

#### I. Fix CORS for Cookies:

```go
func enableCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // IMPORTANT: Can't use "*" with credentials
        origin := os.Getenv("FRONTEND_URL")
        if origin == "" {
            origin = "http://localhost:3000"
        }
        
        w.Header().Set("Access-Control-Allow-Origin", origin)
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

#### A. Update API Calls to Include Credentials:

```typescript
// In UserInterface.tsx or wherever you make API calls

const apiCall = async (url: string, options: RequestInit = {}) => {
  const response = await fetch(url, {
    ...options,
    credentials: 'include', // CRITICAL: This sends cookies
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  });
  
  // If access token expired, try to refresh
  if (response.status === 401) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      // Retry original request
      return fetch(url, {
        ...options,
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          ...options.headers,
        },
      });
    }
  }
  
  return response;
};

// Token refresh function
const refreshAccessToken = async (): Promise<boolean> => {
  try {
    const response = await fetch(`${apiUrl}/api/go/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    return response.ok;
  } catch {
    return false;
  }
};
```

#### B. Replace sessionStorage with Cookie-based Auth:

```typescript
// Remove sessionStorage usage
// Frontend doesn't need to store tokens - they're in HTTP-only cookies

// Just check auth status from backend
const checkAuth = async (): Promise<boolean> => {
  try {
    const response = await fetch(`${apiUrl}/api/go/auth/me`, {
      credentials: 'include',
    });
    return response.ok;
  } catch {
    return false;
  }
};
```

#### C. Update Logout Function:

```typescript
const handleLogout = async () => {
  await fetch(`${apiUrl}/api/go/auth/logout`, {
    method: 'POST',
    credentials: 'include',
  });
  
  router.replace('/');
};
```

### 3. Database Schema

Add table for refresh tokens:

```sql
CREATE TABLE refresh_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    revoked BOOLEAN DEFAULT FALSE
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_token ON refresh_tokens(token);
```

### 4. Environment Variables

Add to `.env`:

```env
JWT_SECRET=your-super-secret-key-change-this-in-production
TOKEN_EXPIRY_MINUTES=15
REFRESH_TOKEN_EXPIRY_DAYS=7
```

## Summary: What You Need

| Feature | Current | Required |
|---------|---------|----------|
| Access Tokens | ❌ No | ✅ JWT in HTTP-only cookie (15 min) |
| Refresh Tokens | ⚠️ Cookie set but unused | ✅ JWT in HTTP-only cookie (7 days) |
| Token Storage | ❌ sessionStorage | ✅ HTTP-only cookies |
| Token Validation | ❌ None | ✅ Middleware on all protected routes |
| Token Refresh | ❌ None | ✅ Auto-refresh before expiry |
| CORS with Credentials | ❌ `*` blocks cookies | ✅ Specific origin + credentials:true |
| Logout | ❌ Client-side only | ✅ Server revokes tokens |
| Protected Routes | ❌ Open to all | ✅ Require valid access token |

## TL;DR - Quick Answer

**Is the current refresh token cookie enough?** 

**NO.** You're setting it but:
1. ❌ Never validating it
2. ❌ Never using it to refresh access
3. ❌ Frontend ignores it (uses sessionStorage)
4. ❌ Backend routes are unprotected

**You need to implement:**
1. ✅ JWT access tokens (short-lived)
2. ✅ JWT refresh tokens (long-lived)  
3. ✅ Authentication middleware
4. ✅ Token refresh endpoint
5. ✅ Fix CORS for credentials
6. ✅ Update frontend to use cookies

This is a significant implementation - probably 4-6 hours of work for a proper, secure setup.



