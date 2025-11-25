import React from 'react';

interface LoginCardProps {
  children: React.ReactNode;
  logoSrc?: string;
  title?: string;
  subtitle?: string;
}

export const LoginCard: React.FC<LoginCardProps> = ({
  children,
  logoSrc = '/Go-Logo_Yellow.png',
  title = 'Welcome Back',
  subtitle = 'Sign in to continue to your account',
}) => {
  return (
    <div className="flex items-center justify-center min-h-screen bg-gradient-to-br from-blue-50 to-cyan-50">
      <div className="w-full max-w-md p-8 space-y-8 bg-white rounded-xl shadow-2xl">
        {/* Logo */}
        <div className="flex justify-center">
          <img 
            src={logoSrc}
            alt="Logo" 
            className="w-32 h-32 object-contain"
          />
        </div>

        {/* Title */}
        <div className="text-center">
          <h1 className="text-3xl font-bold text-gray-900 mb-2">
            {title}
          </h1>
          <p className="text-gray-600">
            {subtitle}
          </p>
        </div>

        {/* Content */}
        {children}

        {/* Footer */}
        <div className="pt-6 text-center text-xs text-gray-500">
          <p>
            By signing in, you agree to our{' '}
            <a href="#" className="text-cyan-600 hover:text-cyan-700">
              Terms of Service
            </a>{' '}
            and{' '}
            <a href="#" className="text-cyan-600 hover:text-cyan-700">
              Privacy Policy
            </a>
          </p>
        </div>
      </div>
    </div>
  );
};

