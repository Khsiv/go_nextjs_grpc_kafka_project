import React, { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import { LoginCard } from '../components/LoginCard';
import { GoogleLoginButton } from '../components/GoogleLoginButton';
import { EmailPasswordForm } from '../components/EmailPasswordForm';
import { ErrorAlert } from '../components/ErrorAlert';

const Home: React.FC = () => {
  const router = useRouter();
  const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000';
  const [error, setError] = useState<string>('');
  const [mode, setMode] = useState<'signin' | 'signup'>('signin');
  const [checkingSession, setCheckingSession] = useState(true);

  const getUserEmail = (): string | null => {
    const raw = document.cookie
      .split('; ')
      .find(row => row.startsWith('user_email='))
      ?.split('=')[1];
    if (!raw) return null;
    try {
      return decodeURIComponent(raw);
    } catch {
      return raw;
    }
  };

  useEffect(() => {
    if (!router.isReady) return;

    const loginParam = router.query.login as string;
    const emailParam = router.query.email as string;
    const errorParam = router.query.error as string;

    if (loginParam === 'success' && emailParam) {
      localStorage.setItem('showSuccessMessage', 'true');
      router.replace('/dashboard');
      return;
    }

    if (errorParam) {
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
      setCheckingSession(false);
      return;
    }

    const verifySession = async () => {
      const email = getUserEmail();
      if (!email) {
        setCheckingSession(false);
        return;
      }

      try {
        const response = await fetch(`${apiUrl}/api/go/users`, {
          credentials: 'include',
        });
        if (response.ok) {
          router.replace('/dashboard');
          return;
        }
        if (response.status === 401) {
          const refreshResponse = await fetch(`${apiUrl}/api/go/auth/refresh`, {
            method: 'POST',
            credentials: 'include',
          });
          if (refreshResponse.ok) {
            router.replace('/dashboard');
            return;
          }
        }
      } catch (authError) {
        console.error('Auth check failed:', authError);
      }

      setCheckingSession(false);
    };

    verifySession();
  }, [router.isReady, router.query.login, router.query.email, router.query.error, apiUrl, router]);

  const handleGoogleLogin = () => {
    window.location.href = `${apiUrl}/api/go/auth/google`;
  };

  const handleSignedIn = () => {
    localStorage.setItem('showSuccessMessage', 'true');
    router.replace('/dashboard');
  };

  if (checkingSession && !error) {
    return null;
  }

  return (
    <LoginCard
      title={mode === 'signup' ? 'Create an account' : 'Welcome Back'}
      subtitle={
        mode === 'signup'
          ? 'Sign up with your email and a password'
          : 'Sign in to continue to your account'
      }
    >
      <ErrorAlert message={error} />

      <EmailPasswordForm
        apiUrl={apiUrl}
        mode={mode}
        onModeChange={setMode}
        onSuccess={handleSignedIn}
        onError={setError}
      />

      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <div className="w-full border-t border-gray-200" />
        </div>
        <div className="relative flex justify-center text-sm">
          <span className="px-2 bg-white text-gray-500">or</span>
        </div>
      </div>

      <GoogleLoginButton onClick={handleGoogleLogin} />
    </LoginCard>
  );
};

export default Home;
