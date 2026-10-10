package controller

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName   = "session_token"
	sessionTTL          = 7 * 24 * time.Hour
	minPasswordLength   = 8
	maxPasswordLength   = 72 // bcrypt only uses the first 72 bytes
	maxAuthRequestBytes = 1 << 20
)

var (
	emailPattern      = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
	dummyPasswordHash []byte
)

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)
	if err != nil {
		panic("password auth: generate dummy hash: " + err.Error())
	}
	dummyPasswordHash = hash
}

func handleSignup(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email, password, ok := readCredentials(w, r)
		if !ok {
			return
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("hashing password: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Could not create account")
			return
		}

		var accountID int
		err = db.QueryRow(
			`INSERT INTO accounts (email, password_hash) VALUES ($1, $2) RETURNING id`,
			email, string(passwordHash),
		).Scan(&accountID)
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				writeJSONError(w, http.StatusConflict, "An account with that email already exists")
				return
			}
			log.Printf("creating account: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Could not create account")
			return
		}

		if !startSession(db, w, accountID, email) {
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"email":   email,
			"message": "Account created",
		})
	}
}

func handleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email, password, ok := readCredentials(w, r)
		if !ok {
			return
		}

		var accountID int
		var passwordHash string
		err := db.QueryRow(
			`SELECT id, password_hash FROM accounts WHERE email = $1`,
			email,
		).Scan(&accountID, &passwordHash)
		if err == sql.ErrNoRows {
			// Spend the same time a real comparison would, so missing accounts
			// are not obviously faster than wrong passwords.
			bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
			writeJSONError(w, http.StatusUnauthorized, "Invalid email or password")
			return
		}
		if err != nil {
			log.Printf("looking up account: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Could not sign in")
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
			writeJSONError(w, http.StatusUnauthorized, "Invalid email or password")
			return
		}

		if !startSession(db, w, accountID, email) {
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"email":   email,
			"message": "Signed in",
		})
	}
}

// refreshPasswordSession slides the expiry of a still-valid email/password
// session and rewrites its cookies. It reports false when there is nothing
// to refresh, and does not write an error response.
func refreshPasswordSession(db *sql.DB, w http.ResponseWriter, r *http.Request) bool {
	if db == nil {
		return false
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	_, email, err := accountFromSession(db, cookie.Value)
	if err != nil {
		return false
	}

	expiresAt := time.Now().Add(sessionTTL)
	result, err := db.Exec(
		`UPDATE sessions SET expires_at = $1 WHERE token_hash = $2`,
		expiresAt, hashToken(cookie.Value),
	)
	if err != nil {
		log.Printf("extending session: %v", err)
		return false
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return false
	}

	setSessionCookies(w, cookie.Value, email)
	return true
}

func deletePasswordSession(db *sql.DB, r *http.Request) {
	if db == nil {
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return
	}
	if _, err := db.Exec(`DELETE FROM sessions WHERE token_hash = $1`, hashToken(cookie.Value)); err != nil {
		log.Printf("deleting session: %v", err)
	}
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}

func accountFromSession(db *sql.DB, token string) (int, string, error) {
	var accountID int
	var email string
	err := db.QueryRow(`
		SELECT a.id, a.email
		FROM sessions s
		JOIN accounts a ON a.id = s.account_id
		WHERE s.token_hash = $1 AND s.expires_at > NOW()`,
		hashToken(token),
	).Scan(&accountID, &email)
	return accountID, email, err
}

func readCredentials(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)

	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return "", "", false
	}

	email, err := validateCredentials(body.Email, body.Password)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	return email, body.Password, true
}

func validateCredentials(email, password string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if !emailPattern.MatchString(normalized) {
		return "", errors.New("Enter a valid email address")
	}
	if len(password) < minPasswordLength {
		return "", fmt.Errorf("Password must be at least %d characters", minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return "", fmt.Errorf("Password must be at most %d characters", maxPasswordLength)
	}
	return normalized, nil
}

func startSession(db *sql.DB, w http.ResponseWriter, accountID int, email string) bool {
	token, err := createSession(db, accountID)
	if err != nil {
		log.Printf("creating session: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Could not start session")
		return false
	}
	setSessionCookies(w, token, email)
	return true
}

func createSession(db *sql.DB, accountID int) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	_, err := db.Exec(
		`INSERT INTO sessions (token_hash, account_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), accountID, time.Now().Add(sessionTTL),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

func setSessionCookies(w http.ResponseWriter, token, email string) {
	maxAge := int(sessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   false, // localhost; turn on behind HTTPS
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "user_email",
		Value:    email,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: false, // the frontend reads this to show who is signed in
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
