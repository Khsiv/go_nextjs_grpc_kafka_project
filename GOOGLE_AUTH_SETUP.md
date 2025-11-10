# Google OAuth Setup Guide

This guide will help you set up Google OAuth authentication for the application.

## Prerequisites

1. A Google Cloud Platform account
2. Node.js and Go installed on your system

## Step 1: Set up Google Cloud Console

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google+ API:
   - Go to "APIs & Services" > "Library"
   - Search for "Google+ API"
   - Click "Enable"

## Step 2: Create OAuth 2.0 Credentials

1. Go to "APIs & Services" > "Credentials"
2. Click "Create Credentials" > "OAuth client ID"
3. Select "Web application"
4. Configure:
   - **Name**: Your app name
   - **Authorized JavaScript origins**: 
     - `http://localhost:8000`
     - `http://localhost:3000`
   - **Authorized redirect URIs**: 
     - `http://localhost:8000/api/go/auth/google/callback`
5. Click "Create"
6. Copy the **Client ID** and **Client Secret**

## Step 3: Configure Environment Variables

Create a `.env` file in the project root with the following variables:

```env
# Database Configuration
DATABASE_URL=postgresql://user:password@localhost:5432/dbname

# Google OAuth Configuration
GOOGLE_CLIENT_ID=your_google_client_id_here
GOOGLE_CLIENT_SECRET=your_google_client_secret_here
GOOGLE_REDIRECT_URI=http://localhost:8000/api/go/auth/google/callback

# Frontend URL (for OAuth redirects)
FRONTEND_URL=http://localhost:3000

# Next.js Frontend Configuration
NEXT_PUBLIC_API_URL=http://localhost:8000
```

Replace the placeholder values with your actual credentials.

## Step 4: Install Dependencies

### Backend (Go)
```bash
cd backend_go
go mod tidy
```

### Frontend (React/Next.js)
```bash
cd frontend_react
npm install
```

## Step 5: Run the Application

### Start the Backend
```bash
cd backend_go
go run main.go
```

The backend will run on `http://localhost:8000`

### Start the Frontend
```bash
cd frontend_react
npm run dev
```

The frontend will run on `http://localhost:3000`

## Step 6: Test the Login

1. Navigate to `http://localhost:3000/login`
2. Click the "Continue with Google" button
3. You'll be redirected to Google's login page
4. After successful authentication, you'll be redirected back to the home page
5. A success message will appear showing your email

## API Endpoints

### Google OAuth Endpoints

- **GET** `/api/go/auth/google` - Initiates Google OAuth flow
- **GET** `/api/go/auth/google/callback` - Handles Google OAuth callback

### User Management Endpoints

- **GET** `/api/go/users` - Get all users
- **POST** `/api/go/users` - Create a new user
- **GET** `/api/go/users/{id}` - Get a specific user
- **PUT** `/api/go/users/{id}` - Update a user
- **DELETE** `/api/go/users/{id}` - Delete a user

## Frontend Pages

- `/` - Home page with user management interface
- `/login` - Login page with Google OAuth button

## Troubleshooting

### "Invalid state parameter" error
- This can occur if you refresh the login page multiple times
- Clear your browser cache and try again

### "Failed to exchange token" error
- Verify your `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` are correct
- Ensure the redirect URI in Google Cloud Console matches exactly

### CORS errors
- Make sure both frontend and backend are running
- Check that `NEXT_PUBLIC_API_URL` in `.env` matches your backend URL

### Database connection errors
- Ensure PostgreSQL is running
- Verify `DATABASE_URL` is correct in `.env`

## Security Notes

1. **Never commit `.env` file** - It's already in `.gitignore`
2. **Use HTTPS in production** - Update all URLs to use HTTPS
3. **Rotate secrets regularly** - Change your OAuth credentials periodically
4. **Implement proper session management** - Consider adding JWT or session tokens
5. **Add rate limiting** - Protect against brute force attacks

## Next Steps

Consider implementing:
- [ ] Session management with JWT tokens
- [ ] User profile page
- [ ] Logout functionality
- [ ] Persistent login state
- [ ] Protected routes
- [ ] User roles and permissions

