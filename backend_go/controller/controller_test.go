package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// TestRouteResolution pins down which route each request path resolves to.
//
// It exists because mux matches in registration order, so "/api/go/users/stats"
// only reaches getAllUserStats while it is registered ahead of "/api/go/users/{id}".
// Reordering those two lines would silently route it to getUser with id="stats",
// and every response would still be a 401 behind the auth middleware — invisible
// until someone logged in and got a 404 instead of their stats.
func TestRouteResolution(t *testing.T) {
	// Handlers close over db/grpcClient but are never invoked here, so nil is safe.
	router := mux.NewRouter()
	registerRoutes(router, nil, nil)

	cases := []struct {
		name     string
		method   string
		path     string
		template string
	}{
		{"batch stats is not treated as a user id", "GET", "/api/go/users/stats", "/api/go/users/stats"},
		{"single user stats", "GET", "/api/go/users/2/stats", "/api/go/users/{id}/stats"},
		{"stats stream", "GET", "/api/go/users/2/stats/stream", "/api/go/users/{id}/stats/stream"},
		{"recalculate", "POST", "/api/go/users/2/stats/recalculate", "/api/go/users/{id}/stats/recalculate"},
		{"existing user route still resolves", "GET", "/api/go/users/2", "/api/go/users/{id}"},
		{"existing posts route still resolves", "GET", "/api/go/users/2/posts", "/api/go/users/{id}/posts"},
		{"existing createPost route still resolves", "POST", "/api/go/users/createPost", "/api/go/users/createPost"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var match mux.RouteMatch
			request := httptest.NewRequest(tc.method, tc.path, http.NoBody)

			if !router.Match(request, &match) {
				t.Fatalf("%s %s matched no route", tc.method, tc.path)
			}

			template, err := match.Route.GetPathTemplate()
			if err != nil {
				t.Fatalf("reading path template: %v", err)
			}

			if template != tc.template {
				t.Errorf("%s %s resolved to %q, want %q", tc.method, tc.path, template, tc.template)
			}
		})
	}
}
