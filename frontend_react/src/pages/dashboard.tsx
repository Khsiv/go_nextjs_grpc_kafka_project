import React, { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import { ProtectedContent } from '../components/ProtectedContent';

const Dashboard: React.FC = () => {
  const router = useRouter();
  const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000';
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [userEmail, setUserEmail] = useState<string>('');
  const [successMessage, setSuccessMessage] = useState<string>('');

  // Get email from cookie
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

  // Check authentication by validating cookie
  const checkAuth = async (): Promise<boolean> => {
    try {
      // Try to fetch users to validate access token
      const response = await fetch(`${apiUrl}/api/go/users`, {
        credentials: 'include',
      });

      if (response.ok) {
        return true;
      }

      // If 401, try to refresh token
      if (response.status === 401) {
        const refreshResponse = await fetch(`${apiUrl}/api/go/auth/refresh`, {
          method: 'POST',
          credentials: 'include',
        });
        return refreshResponse.ok;
      }

      return false;
    } catch (error) {
      console.error('Auth check failed:', error);
      return false;
    }
  };

  useEffect(() => {
    const initAuth = async () => {
      const email = getUserEmail();
      const isAuth = await checkAuth();

      if (isAuth && email) {
        setIsAuthenticated(true);
        setUserEmail(email);

        // Check if we should show success message
        const showSuccess = localStorage.getItem('showSuccessMessage');
        if (showSuccess === 'true') {
          setSuccessMessage(`Successfully logged in as ${email}`);
          localStorage.removeItem('showSuccessMessage');

          // Clear the success message after 5 seconds
          const timer = setTimeout(() => {
            setSuccessMessage('');
          }, 5000);

          return () => clearTimeout(timer);
        }
      } else {
        // Not authenticated, redirect to home (sign-in page)
        router.replace('/');
      }
    };

    initAuth();
  }, [router, apiUrl]);

  const handleLogout = async () => {
    try {
      // Call logout endpoint to revoke tokens
      await fetch(`${apiUrl}/api/go/auth/logout`, {
        method: 'POST',
        credentials: 'include',
      });
    } catch (error) {
      console.error('Logout error:', error);
    } finally {
      setIsAuthenticated(false);
      setUserEmail('');
      // Redirect to home (sign-in page)
      router.replace('/');
    }
  };

  // Show loading or nothing while checking auth
  if (!isAuthenticated) {
    return null;
  }

  return (
    <>
      {/* Success Message Banner */}
      {successMessage && (
        <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50 w-full max-w-md">
          <div className="bg-green-500 text-white px-6 py-4 rounded-lg shadow-lg flex items-center justify-between">
            <div className="flex items-center gap-3">
              <svg 
                className="w-6 h-6" 
                fill="none" 
                stroke="currentColor" 
                viewBox="0 0 24 24"
              >
                <path 
                  strokeLinecap="round" 
                  strokeLinejoin="round" 
                  strokeWidth={2} 
                  d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" 
                />
              </svg>
              <span className="font-medium">{successMessage}</span>
            </div>
            <button 
              onClick={() => setSuccessMessage('')}
              className="text-white hover:text-gray-200 transition-colors"
            >
              <svg 
                className="w-5 h-5" 
                fill="none" 
                stroke="currentColor" 
                viewBox="0 0 24 24"
              >
                <path 
                  strokeLinecap="round" 
                  strokeLinejoin="round" 
                  strokeWidth={2} 
                  d="M6 18L18 6M6 6l12 12" 
                />
              </svg>
            </button>
          </div>
        </div>
      )}
      
      <ProtectedContent userEmail={userEmail} onLogout={handleLogout} />
    </>
  );
};

export default Dashboard;

