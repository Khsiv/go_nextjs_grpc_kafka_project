import React, { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import UserInterface from '../components/UserInterface';

const Home: React.FC = () => {
  const router = useRouter();
  const [successMessage, setSuccessMessage] = useState<string>('');

  useEffect(() => {
    // Check for login success in URL query params
    const loginParam = router.query.login as string;
    const emailParam = router.query.email as string;
    
    if (loginParam === 'success' && emailParam) {
      setSuccessMessage(`Successfully logged in as ${emailParam}`);
      
      // Clear the success message after 5 seconds
      const timer = setTimeout(() => {
        setSuccessMessage('');
        // Remove query params from URL
        router.replace('/', undefined, { shallow: true });
      }, 5000);

      return () => clearTimeout(timer);
    }
  }, [router]);

  return (
    <main className="flex flex-wrap justify-center items-start min-h-screen bg-gray-100">
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

      <div className="m-4">
        <UserInterface backendName="go" />
      </div>
    </main>
  );
};

export default Home;