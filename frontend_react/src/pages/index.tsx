import React, { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import { LoginCard } from '../components/LoginCard';
import { GoogleLoginButton } from '../components/GoogleLoginButton';
import { ErrorAlert } from '../components/ErrorAlert';

const Home: React.FC = () => {
  const router = useRouter();
  const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000';
  const [error, setError] = useState<string>('');

  // Get email from cookie
  const getUserEmail = (): string | null => {
    const email = document.cookie
      .split('; ')
      .find(row => row.startsWith('user_email='))
      ?.split('=')[1];
    return email || null;
  };

  // Check if access token cookie exists
  const hasAccessToken = (): boolean => {
    return document.cookie.includes('google_access_token=');
  };

  useEffect(() => {
    // Check for login success in URL query params
    const loginParam = router.query.login as string;
    const emailParam = router.query.email as string;
    const errorParam = router.query.error as string;
    
    if (loginParam === 'success' && emailParam) {
      // Set flag to show success message on dashboard
      localStorage.setItem('showSuccessMessage', 'true');
      
      // Redirect to dashboard
      router.replace('/dashboard');
    } else if (errorParam) {
      // Handle various error types
      switch (errorParam) {
        case 'verification_failed':
          setError('Email verification failed. Please try again.');
          break;
        case 'invalid_state':
          setError('Invalid authentication state. Please try again.');
          break;
        case 'token_exchange_failed':
          setError('Failed to exchange token. Please try again.');
          break;
        default:
          setError('Authentication failed. Please try again.');
      }
    } else {
      // Check if user is already authenticated (has cookies)
      const email = getUserEmail();
      const hasToken = hasAccessToken();
      
      if (email && hasToken) {
        // Already logged in, redirect to dashboard
        router.replace('/dashboard');
      }
    }
  }, [router.query, router]);

  const handleGoogleLogin = () => {
    // Redirect to the backend Google OAuth endpoint
    window.location.href = `${apiUrl}/api/go/auth/google`;
  };

  return (
    <LoginCard>
      <ErrorAlert message={error} />
      
      <div className="space-y-4">
        <GoogleLoginButton onClick={handleGoogleLogin} />
      </div>
    </LoginCard>
  );
};

export default Home;