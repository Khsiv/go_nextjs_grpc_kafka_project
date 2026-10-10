import React, { useState } from 'react';

type AuthMode = 'signin' | 'signup';

interface EmailPasswordFormProps {
  apiUrl: string;
  mode: AuthMode;
  onModeChange: (mode: AuthMode) => void;
  onSuccess: (email: string) => void;
  onError: (message: string) => void;
}

export const EmailPasswordForm: React.FC<EmailPasswordFormProps> = ({
  apiUrl,
  mode,
  onModeChange,
  onSuccess,
  onError,
}) => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const isSignup = mode === 'signup';

  const switchMode = (next: AuthMode) => {
    onError('');
    setPassword('');
    setConfirmPassword('');
    onModeChange(next);
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onError('');

    if (isSignup && password !== confirmPassword) {
      onError('Passwords do not match.');
      return;
    }

    setSubmitting(true);
    try {
      const response = await fetch(`${apiUrl}/api/go/auth/${isSignup ? 'signup' : 'login'}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });

      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        onError(data.error || 'Something went wrong. Please try again.');
        return;
      }

      onSuccess(typeof data.email === 'string' ? data.email : email);
    } catch {
      onError('Could not reach the server. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label htmlFor="email" className="block text-sm font-medium text-gray-700 mb-1">
          Email
        </label>
        <input
          id="email"
          name="email"
          type="email"
          autoComplete="email"
          required
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"
        />
      </div>

      <div>
        <label htmlFor="password" className="block text-sm font-medium text-gray-700 mb-1">
          Password
        </label>
        <input
          id="password"
          name="password"
          type="password"
          autoComplete={isSignup ? 'new-password' : 'current-password'}
          required
          minLength={8}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"
        />
        {isSignup && (
          <p className="mt-1 text-xs text-gray-500">At least 8 characters.</p>
        )}
      </div>

      {isSignup && (
        <div>
          <label htmlFor="confirm-password" className="block text-sm font-medium text-gray-700 mb-1">
            Confirm password
          </label>
          <input
            id="confirm-password"
            name="confirm-password"
            type="password"
            autoComplete="new-password"
            required
            minLength={8}
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"
          />
        </div>
      )}

      <button
        type="submit"
        disabled={submitting}
        className="w-full px-4 py-3 text-white bg-cyan-600 rounded-lg hover:bg-cyan-700 transition-colors font-medium disabled:opacity-60 disabled:cursor-not-allowed"
      >
        {submitting ? 'Please wait…' : isSignup ? 'Create account' : 'Sign in'}
      </button>

      <p className="text-center text-sm text-gray-600">
        {isSignup ? 'Already have an account?' : 'Need an account?'}{' '}
        <button
          type="button"
          onClick={() => switchMode(isSignup ? 'signin' : 'signup')}
          className="text-cyan-700 hover:text-cyan-800 font-medium"
        >
          {isSignup ? 'Sign in' : 'Sign up'}
        </button>
      </p>
    </form>
  );
};
