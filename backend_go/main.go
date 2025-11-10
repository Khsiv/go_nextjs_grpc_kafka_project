package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type User struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Description string `json:"description"`
}

type OAuthData struct {
	Id             string `json:"id"`
	Email          string `json:"email"`
	Verified_email bool   `json:"verified_email"`
	Picture        string `json:"picture"`
}

var (
	oauthStateCache = make(map[string]bool)
	conf            *oauth2.Config
)

// main function
func main() {
	// connect to database
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// create table if it doesn't exist
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, name TEXT, email TEXT, description TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	// initialize OAuth
	initOAuth()

	// create router
	router := mux.NewRouter()
	router.HandleFunc("/api/go/users", getUsers(db)).Methods("GET")
	router.HandleFunc("/api/go/users", createUser(db)).Methods("POST")
	router.HandleFunc("/api/go/users/{id}", getUser(db)).Methods("GET")
	router.HandleFunc("/api/go/users/{id}", updateUser(db)).Methods("PUT")
	router.HandleFunc("/api/go/users/{id}", deleteUser(db)).Methods("DELETE")

	router.HandleFunc("/api/go/auth/google", redirectToGoogleLogin).Methods("GET")
	router.HandleFunc("/api/go/auth/google/callback", handleGoogleCallback).Methods("GET")

	// wrap the router with CORS and JSON content type middlewares
	enhancedRouter := enableCORS(jsonContentTypeMiddleware(router))
	// start server
	log.Fatal(http.ListenAndServe(":8000", enhancedRouter))
}

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

func redirectToGoogleLogin(w http.ResponseWriter, r *http.Request) {
	state := make([]byte, 32)

	_, err := rand.Read(state)

	if _, ok := oauthStateCache[string(state)]; ok {
		log.Fatalf("State already exists")
	}

	oauthStateCache[string(state)] = true

	if err != nil {
		log.Fatalf("Error generating random bytes: %v", err)
	}

	fmt.Printf("client id: %s\n", conf.ClientID)
	fmt.Printf("client secret: %s\n", conf.ClientSecret)
	fmt.Printf("redirect url: %s\n", conf.RedirectURL)
	fmt.Printf("scopes: %s\n", conf.Scopes)
	fmt.Printf("endpoint: %s\n", conf.Endpoint)

	url := conf.AuthCodeURL(string(state), oauth2.AccessTypeOffline)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")

	if _, ok := oauthStateCache[state]; !ok {
		log.Fatalf("Invalid state: %s", state)
		return
	}
	delete(oauthStateCache, state)

	code := r.URL.Query().Get("code")

	token, err := conf.Exchange(context.Background(), code)

	if err != nil {
		log.Fatalf("Error exchanging code for token: %v", err)
		return
	}

	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)

	if err != nil {
		log.Fatalf("Error getting user info: %v", err)
		return
	}

	data, err := io.ReadAll(response.Body)

	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
		return
	}

	var oAuthData OAuthData
	err = json.Unmarshal(data, &oAuthData)

	if err != nil {
		log.Fatalf("Error unmarshalling user info: %v", err)
		return
	}

	status := oAuthData.Verified_email
	if status {
		fmt.Fprintf(w, "success: %s is a verified user\n", oAuthData.Email)
	} else {
		fmt.Fprint(w, "failed verification")
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	if oAuthData.Verified_email {
		// Success - redirect to home page with success message
		http.Redirect(w, r, fmt.Sprintf("%s/?login=success&email=%s", frontendURL, oAuthData.Email), http.StatusTemporaryRedirect)
	} else {
		// Failed - redirect to login page with error
		http.Redirect(w, r, fmt.Sprintf("%s/login?error=verification_failed", frontendURL), http.StatusTemporaryRedirect)
	}
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow any origin
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

		users := []User{} // array of users
		for rows.Next() {
			var u User
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

// get user by id
func getUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u User
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

// create user
func createUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u User
		json.NewDecoder(r.Body).Decode(&u)

		err := db.QueryRow("INSERT INTO users (name, email, description) VALUES ($1, $2, $3) RETURNING id", u.Name, u.Email, u.Description).Scan(&u.Id)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("User created: ", u)

		json.NewEncoder(w).Encode(u)
	}
}

// update user
func updateUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u User
		json.NewDecoder(r.Body).Decode(&u)

		vars := mux.Vars(r)
		id := vars["id"]

		// Execute the update query
		_, err := db.Exec("UPDATE users SET name = $1, email = $2, description = $3 WHERE id = $4", u.Name, u.Email, u.Description, id)
		if err != nil {
			log.Fatal(err)
		}

		// Retrieve the updated user data from the database
		var updatedUser User
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

		// Send the updated user data in the response
		json.NewEncoder(w).Encode(updatedUser)
	}
}

// delete user
func deleteUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u User
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

			json.NewEncoder(w).Encode("User deleted")
		}
	}
}
