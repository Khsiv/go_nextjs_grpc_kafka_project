import React, { useEffect, useState } from 'react';
import { useRouter } from 'next/router';

interface Post {
  id: number;
  userId: number;
  content: string;
}

const UserPostsPage: React.FC = () => {
  const router = useRouter();
  const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000';
  const [userEmail, setUserEmail] = useState('');
  const [posts, setPosts] = useState<Post[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');

  const userId = typeof router.query.id === 'string' ? router.query.id : '';

  const getUserEmail = (): string | null => {
    const raw = document.cookie
      .split('; ')
      .find((row) => row.startsWith('user_email='))
      ?.split('=')[1];
    if (!raw) return null;
    try {
      return decodeURIComponent(raw);
    } catch {
      return raw;
    }
  };

  const refreshAccessToken = async (): Promise<boolean> => {
    try {
      const response = await fetch(`${apiUrl}/api/go/auth/refresh`, {
        method: 'POST',
        credentials: 'include',
      });
      return response.ok;
    } catch (refreshError) {
      console.error('Token refresh failed:', refreshError);
      return false;
    }
  };

  const apiCall = async (url: string, options: RequestInit = {}): Promise<Response> => {
    const response = await fetch(url, {
      ...options,
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
    });

    if (response.status === 401) {
      const refreshed = await refreshAccessToken();
      if (refreshed) {
        return fetch(url, {
          ...options,
          credentials: 'include',
          headers: {
            'Content-Type': 'application/json',
            ...options.headers,
          },
        });
      }

      window.location.href = '/';
      throw new Error('Session expired');
    }

    return response;
  };

  const checkAuth = async (): Promise<boolean> => {
    try {
      const response = await fetch(`${apiUrl}/api/go/users`, {
        credentials: 'include',
      });

      if (response.ok) {
        return true;
      }

      if (response.status === 401) {
        const refreshResponse = await fetch(`${apiUrl}/api/go/auth/refresh`, {
          method: 'POST',
          credentials: 'include',
        });
        return refreshResponse.ok;
      }

      return false;
    } catch (authError) {
      console.error('Auth check failed:', authError);
      return false;
    }
  };

  const handleLogout = async () => {
    try {
      await fetch(`${apiUrl}/api/go/auth/logout`, {
        method: 'POST',
        credentials: 'include',
      });
    } catch (logoutError) {
      console.error('Logout error:', logoutError);
    } finally {
      router.replace('/');
    }
  };

  useEffect(() => {
    if (!router.isReady || !userId) {
      return;
    }

    const loadPosts = async () => {
      setIsLoading(true);
      setError('');

      const email = getUserEmail();
      const isAuthenticated = await checkAuth();

      if (!isAuthenticated || !email) {
        router.replace('/');
        return;
      }

      setUserEmail(email);

      try {
        const response = await apiCall(`${apiUrl}/api/go/users/${userId}/posts`);
        const data = await response.json().catch(() => []);

        if (!response.ok) {
          setError(data?.error || 'Failed to load posts.');
          return;
        }

        setPosts(Array.isArray(data) ? data : []);
      } catch (loadError) {
        console.error('Error fetching posts:', loadError);
        setError('Failed to load posts.');
      } finally {
        setIsLoading(false);
      }
    };

    loadPosts();
  }, [apiUrl, router, userId]);

  if (!router.isReady || isLoading) {
    return (
      <main className="min-h-screen bg-gray-100 p-6">
        <div className="mx-auto max-w-4xl rounded-lg bg-white p-6 shadow">
          <p className="text-gray-700">Loading posts...</p>
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-gray-100">
      <div className="w-full bg-white shadow-md">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-4">
          <div>
            <h1 className="text-xl font-bold text-gray-900">Posts For User {userId}</h1>
            <p className="text-sm text-gray-600">Logged in as {userEmail}</p>
          </div>
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => router.push('/dashboard')}
              className="rounded-lg bg-gray-600 px-4 py-2 text-sm font-medium text-white hover:bg-gray-700"
            >
              Back To Dashboard
            </button>
            <button
              type="button"
              onClick={handleLogout}
              className="rounded-lg bg-red-500 px-4 py-2 text-sm font-medium text-white hover:bg-red-600"
            >
              Logout
            </button>
          </div>
        </div>
      </div>

      <div className="mx-auto max-w-4xl p-6">
        {error ? (
          <div className="mb-4 rounded-lg bg-red-100 p-4 text-red-700">{error}</div>
        ) : null}

        {!error && posts.length === 0 ? (
          <div className="rounded-lg bg-white p-6 shadow">
            <p className="text-gray-700">No posts found for this user yet.</p>
          </div>
        ) : null}

        <div className="space-y-4">
          {posts.map((post) => (
            <article key={post.id} className="rounded-lg bg-white p-6 shadow">
              <div className="mb-2 text-sm text-gray-500">Post Id: {post.id}</div>
              <div className="text-sm text-gray-500">User Id: {post.userId}</div>
              <p className="mt-4 whitespace-pre-wrap text-gray-800">{post.content}</p>
            </article>
          ))}
        </div>
      </div>
    </main>
  );
};

export default UserPostsPage;
