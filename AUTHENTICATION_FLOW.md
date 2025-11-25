# Authentication Flow Guide

## Overview
This document describes the authentication flow for the Go Backend application with Google OAuth integration.

## User Flow

### 1. Sign-In Page (`/`)
- **When:** User visits the site for the first time or is not logged in
- **What user sees:**
  - "Welcome Back" message with logo
  - "Continue with Google" button
  - Terms of Service and Privacy Policy links
- **Action:** Click "Continue with Google" → Initiates OAuth flow

### 2. Google OAuth Flow
- User is redirected to Google's login page
- User authenticates with Google
- Google redirects back to backend callback
- Backend verifies the user and redirects to frontend

### 3. Dashboard (`/dashboard`)
- **When:** User successfully completes Google OAuth (auto-redirected from home)
- **What user sees:**
  - Success message banner (auto-dismisses after 5 seconds)
  - Header with logo and user email
  - Logout button
  - User management interface (Add User, Update User, Delete User)
- **Actions available:**
  - Manage users (CRUD operations)
  - Logout → Returns to sign-in page (`/`)

## Authentication State Management

### Session Storage
The application uses `sessionStorage` to maintain authentication state:
- `isAuthenticated`: Boolean flag
- `userEmail`: Logged-in user's email

### Why Session Storage?
- Persists across page refreshes
- Cleared when browser tab/window is closed
- Client-side only (appropriate for this demo app)

## Components

### New Components Created

1. **`ProtectedContent.tsx`**
   - Displays user management dashboard
   - Shows user email in header
   - Includes logout functionality
   - Only visible to authenticated users

3. **`LoginCard.tsx`**
   - Reusable card wrapper for login pages
   - Includes logo, title, subtitle, and footer
   - Configurable with props

4. **`GoogleLoginButton.tsx`**
   - Reusable Google OAuth button
   - Includes Google logo SVG
   - Accepts onClick handler

5. **`ErrorAlert.tsx`**
   - Displays authentication errors
   - Auto-hides when no error

### Pages

1. **`index.tsx` (Home / Sign-in Page)**
   - Shows sign-in form with Google OAuth button
   - Handles OAuth callback and stores authentication
   - Redirects to `/dashboard` after successful login
   - Redirects to `/dashboard` if already authenticated
   - Handles authentication errors

2. **`dashboard.tsx` (Dashboard Page)**
   - Protected route - requires authentication
   - Shows user management interface
   - Displays logged-in user email in header
   - Implements logout functionality
   - Redirects to `/` if not authenticated

## Security Notes

⚠️ **Important:** This is a demo authentication flow. For production applications, you should:

1. Use HTTP-only cookies instead of sessionStorage
2. Implement JWT tokens with refresh tokens
3. Add CSRF protection
4. Use server-side session management
5. Implement proper user roles and permissions
6. Add rate limiting for login attempts
7. Use HTTPS in production
8. Implement proper error handling and logging

## Testing the Flow

1. Visit `http://localhost:3000/`
2. You should see the sign-in page with "Continue with Google" button
3. Click "Continue with Google"
4. Complete Google authentication
5. You'll be redirected to `http://localhost:3000/dashboard`
6. Your email will be displayed in the header
7. You can manage users (Add, Update, Delete)
8. Click "Logout" to return to the sign-in page
9. If you try to visit `/dashboard` without logging in, you'll be redirected to `/`

## Environment Variables Required

Make sure these are set in your `.env` file:

```env
# Frontend
NEXT_PUBLIC_API_URL=http://localhost:8000

# Backend
GOOGLE_CLIENT_ID=your_google_client_id
GOOGLE_CLIENT_SECRET=your_google_client_secret
GOOGLE_REDIRECT_URL=http://localhost:8000/api/go/auth/google/callback
FRONTEND_URL=http://localhost:3000
DATABASE_URL=postgresql://user:password@db:5432/dbname
```

## Architecture Diagram

```
┌─────────────────┐
│   Sign-In Page  │ (Not Authenticated)
│        /        │
│                 │
│  - Welcome Back │
│  - Google Login │
└────────┬────────┘
         │ Click "Continue with Google"
         ▼
┌─────────────────┐
│  Google OAuth   │
│   (External)    │
└────────┬────────┘
         │ OAuth Success
         ▼
┌─────────────────┐
│ Backend Callback│
│  /auth/google/  │
│    callback     │
└────────┬────────┘
         │ Verify & Redirect to /
         ▼
┌─────────────────┐
│        /        │
│  (Detect login  │
│   & redirect)   │
└────────┬────────┘
         │ Auto-redirect
         ▼
┌─────────────────┐
│   Dashboard     │ (Authenticated)
│   /dashboard    │
│                 │
│ - User Email    │
│ - User CRUD     │
│ - Logout        │
└─────────────────┘
         │ Click Logout
         ▼
┌─────────────────┐
│   Sign-In Page  │ (Back to start)
│        /        │
└─────────────────┘
```

## Files Modified/Created

### Created:
- `frontend_react/src/pages/dashboard.tsx` - Protected dashboard page
- `frontend_react/src/components/ProtectedContent.tsx`
- `frontend_react/src/components/LoginCard.tsx`
- `frontend_react/src/components/GoogleLoginButton.tsx`
- `frontend_react/src/components/ErrorAlert.tsx`

### Modified:
- `frontend_react/src/pages/index.tsx` - Sign-in page with redirect to dashboard
- `frontend_react/src/components/UserInterface.tsx` - Removed "Go to Login Page" button
- `backend_go/main.go` - Updated error redirect to home page instead of /login

### Deleted:
- `frontend_react/src/pages/login.tsx` - No longer needed (merged into home page)
- `frontend_react/src/components/LandingPage.tsx` - No longer needed

## Next Steps / Future Enhancements

1. Add password-based authentication option
2. Implement "Remember Me" functionality
3. Add user profile management
4. Implement role-based access control
5. Add password reset functionality
6. Create a signup page for new users
7. Add email verification step
8. Implement two-factor authentication

