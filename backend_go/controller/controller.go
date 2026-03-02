package controller

import (
	"api/models"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	oauthStateCache = make(map[string]bool)
	conf            *oauth2.Config
)

func initOAuth() {
	// Initialize Google OAuth config
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

	fmt.Println("ClientID: ", conf.ClientID)
	fmt.Println("ClientSecret: ", conf.ClientSecret)

	if conf.ClientID == "" || conf.ClientSecret == "" {
		log.Println("Warning: Google OAuth credentials not set. Please set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET environment variables.")
	}
}

func initDatabase() *sql.DB {
	// connect to database
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable"
		log.Println("DATABASE_URL not set, using default local connection:", connStr)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	return db
}

func GetRouter() *mux.Router {

	router := mux.NewRouter()

	initOAuth()

	db := initDatabase()
	defer db.Close()

	// PUBLIC routes (no authentication required)
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

	// wrap the router with CORS and JSON content type middlewares
	enhancedRouter := enableCORS(jsonContentTypeMiddleware(router))
	// start server
	log.Fatal(http.ListenAndServe(":8000", enhancedRouter))

	return router
}

// Helper function to get frontend URL
func getFrontendURL() string {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	return frontendURL
}

func redirectToGoogleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate random bytes for state
	stateBytes := make([]byte, 32)
	_, err := rand.Read(stateBytes)
	if err != nil {
		log.Printf("Error generating random bytes: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Encode to base64 URL-safe string
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Check for duplicate (very unlikely with 32 random bytes)
	if _, ok := oauthStateCache[state]; ok {
		log.Printf("Warning: State collision detected, regenerating...")
	}

	// Store state in cache
	oauthStateCache[state] = true
	log.Printf("Generated OAuth state: %s", state)

	// Generate OAuth URL with offline access to get refresh token
	url := conf.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce, // Force prompt to ensure we get refresh token
	)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")

	// Validate state
	if _, ok := oauthStateCache[state]; !ok {
		log.Printf("Invalid state: %s", state)
		frontendURL := getFrontendURL()
		http.Redirect(w, r, fmt.Sprintf("%s/?error=invalid_state", frontendURL), http.StatusTemporaryRedirect)
		return
	}
	delete(oauthStateCache, state)
	log.Printf("State validated and removed from cache")

	code := r.URL.Query().Get("code")

	// Exchange code for token (includes access_token and refresh_token)
	token, err := conf.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("Error exchanging code for token: %v", err)
		frontendURL := getFrontendURL()
		http.Redirect(w, r, fmt.Sprintf("%s/?error=token_exchange_failed", frontendURL), http.StatusTemporaryRedirect)
		return
	}

	// Get models.User info using access token
	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		log.Printf("Error getting models.User info: %v", err)
		frontendURL := getFrontendURL()
		http.Redirect(w, r, fmt.Sprintf("%s/?error=userinfo_failed", frontendURL), http.StatusTemporaryRedirect)
		return
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}

	var oAuthData models.OAuthData
	err = json.Unmarshal(data, &oAuthData)
	if err != nil {
		log.Printf("Error unmarshalling models.User info: %v", err)
		return
	}

	// Check email verification
	if !oAuthData.Verified_email {
		log.Printf("Email not verified for models.User: %s", oAuthData.Email)
		frontendURL := getFrontendURL()
		http.Redirect(w, r, fmt.Sprintf("%s/?error=verification_failed", frontendURL), http.StatusTemporaryRedirect)
		return
	}

	// Log success
	log.Printf("OAuth successful for models.User: %s", oAuthData.Email)

	// Set Google Access Token cookie (1 hour expiry)
	http.SetCookie(w, &http.Cookie{
		Name:     "google_access_token",
		Value:    token.AccessToken,
		Path:     "/",
		MaxAge:   30, // 1 hour
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	// Set Google Refresh Token cookie (long-lived)
	if token.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:  "google_refresh_token",
			Value: token.RefreshToken,
			Path:  "/",
			// MaxAge:   2592000
			MaxAge:   100,
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		})
		log.Printf("Refresh token set for models.User: %s", oAuthData.Email)
	} else {
		log.Printf("Warning: No refresh token received for models.User: %s", oAuthData.Email)
	}

	// Store models.User email cookie (readable by frontend)
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
	frontendURL := getFrontendURL()
	http.Redirect(w, r, fmt.Sprintf("%s/?login=success&email=%s", frontendURL, oAuthData.Email), http.StatusTemporaryRedirect)
}

// Validate Google access token
func validateGoogleToken(accessToken string) (*models.OAuthData, error) {
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

	var userInfo models.OAuthData
	err = json.Unmarshal(data, &userInfo)
	if err != nil {
		return nil, err
	}

	return &userInfo, nil
}

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
			log.Printf("Access token invalid: %v", err)
			http.Error(w, `{"error": "Unauthorized - Invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Add models.User info to context
		ctx := context.WithValue(r.Context(), "user_email", userInfo.Email)
		ctx = context.WithValue(ctx, "user_id", userInfo.Id)

		// Call next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// Token refresh endpoint
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
		MaxAge:   100,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	// Google may return a new refresh token - update it if provided
	if newToken.RefreshToken != "" {
		log.Printf("New refresh token: %s", newToken.RefreshToken)
		http.SetCookie(w, &http.Cookie{
			Name:     "google_refresh_token",
			Value:    newToken.RefreshToken,
			Path:     "/",
			MaxAge:   100,
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		})
		log.Printf("Refresh token also updated")
	}

	log.Printf("Token refreshed successfully")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Token refreshed successfully",
	})
}

// Logout endpoint
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
		Name:   "user_email",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	log.Printf("models.User logged out successfully")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Logged out successfully",
	})
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get frontend URL
		frontendURL := getFrontendURL()

		// CRITICAL: Must use specific origin, not "*" for cookies
		w.Header().Set("Access-Control-Allow-Origin", frontendURL)
		w.Header().Set("Access-Control-Allow-Credentials", "true") // REQUIRED for cookies
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Check if the request is for CORS preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, r)
	})
}

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set JSON Content-Type
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// get all users
func getUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, name, email, description FROM users")
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()

		users := []models.User{} // array of users
		for rows.Next() {
			var u models.User
			var desc sql.NullString
			if err := rows.Scan(&u.Id, &u.Name, &u.Email, &desc); err != nil {
				log.Fatal(err)
			}
			if desc.Valid {
				u.Description = desc.String
			} else {
				u.Description = ""
			}
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("in Users")

		json.NewEncoder(w).Encode(users)

	}

}

// get models.User by id
func getUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u models.User
		var desc sql.NullString
		err := db.QueryRow("SELECT id, name, email, description FROM users WHERE id = $1", id).Scan(&u.Id, &u.Name, &u.Email, &desc)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if desc.Valid {
			u.Description = desc.String
		} else {
			u.Description = ""
		}

		json.NewEncoder(w).Encode(u)
	}
}

// create models.User
func createUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u models.User
		json.NewDecoder(r.Body).Decode(&u)

		err := db.QueryRow("INSERT INTO users (name, email, description) VALUES ($1, $2, $3) RETURNING id", u.Name, u.Email, u.Description).Scan(&u.Id)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("models.User created: ", u)

		json.NewEncoder(w).Encode(u)
	}
}

// update models.User
func updateUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u models.User
		json.NewDecoder(r.Body).Decode(&u)

		vars := mux.Vars(r)
		id := vars["id"]

		// Execute the update query
		_, err := db.Exec("UPDATE users SET name = $1, email = $2, description = $3 WHERE id = $4", u.Name, u.Email, u.Description, id)
		if err != nil {
			log.Fatal(err)
		}

		// Retrieve the updated models.User data from the database
		var updatedUser models.User
		var desc sql.NullString
		err = db.QueryRow("SELECT id, name, email, description FROM users WHERE id = $1", id).Scan(&updatedUser.Id, &updatedUser.Name, &updatedUser.Email, &desc)
		if err != nil {
			log.Fatal(err)
		}
		if desc.Valid {
			updatedUser.Description = desc.String
		} else {
			updatedUser.Description = ""
		}

		// Send the updated models.User data in the response
		json.NewEncoder(w).Encode(updatedUser)
	}
}

// delete models.User
func deleteUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u models.User
		err := db.QueryRow("SELECT id FROM users WHERE id = $1", id).Scan(&u.Id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			_, err := db.Exec("DELETE FROM users WHERE id = $1", id)
			if err != nil {
				//todo : fix error handling
				w.WriteHeader(http.StatusNotFound)
				return
			}

			json.NewEncoder(w).Encode("models.User deleted")
		}
	}
}
