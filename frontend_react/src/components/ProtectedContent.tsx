import React from 'react';
import UserInterface from './UserInterface';

interface ProtectedContentProps {
  userEmail: string;
  onLogout: () => void;
}

export const ProtectedContent: React.FC<ProtectedContentProps> = ({ userEmail, onLogout }) => {

  return (
    <main className="flex flex-wrap justify-center items-start min-h-screen bg-gray-100">
      {/* Header with user info and logout */}
      <div className="w-full bg-white shadow-md">
        <div className="max-w-7xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <img 
              src="/Go-Logo_Yellow.png" 
              alt="Logo" 
              className="w-10 h-10 object-contain"
            />
            <div>
              <h2 className="text-xl font-bold text-gray-900">Go Backend</h2>
              <p className="text-sm text-gray-600">User Management Dashboard</p>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <div className="text-right">
              <p className="text-sm text-gray-600">Logged in as</p>
              <p className="text-sm font-semibold text-gray-900">{userEmail}</p>
            </div>
            <button
              onClick={onLogout}
              className="px-4 py-2 text-sm font-medium text-white bg-red-500 rounded-lg hover:bg-red-600 transition-colors"
            >
              Logout
            </button>
          </div>
        </div>
      </div>

      {/* User Management Interface */}
      <div className="m-4">
        <UserInterface backendName="go" />
      </div>
    </main>
  );
};

