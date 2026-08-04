import { useCallback, useEffect, useRef, useState } from 'react';

export type UserStatsStatus = 'never' | 'pending' | 'done' | 'failed';

export interface UserStats {
  user_id: number;
  total_characters: number;
  posts_count: number;
  status: UserStatsStatus;
  request_id?: string;
  error_message?: string;
  calculated_at?: string;
}

// Safety net: if stats_service never answers (broker down, consumer crashed), stop
// spinning instead of leaving the row stuck on "Calculating..." forever.
const RESULT_TIMEOUT_MS = 60_000;

interface OpenStream {
  source: EventSource;
  timeoutId: ReturnType<typeof setTimeout>;
  /** Guards against EventSource's automatic reconnect re-publishing the request. */
  requested: boolean;
}

/**
 * Drives the asynchronous character-count feature.
 *
 * Pressing the button opens an SSE stream first and only publishes the Kafka request
 * once the stream is actually open, so the result can never arrive before anyone is
 * listening for it.
 */
export function useUserStats(apiUrl: string, backendName: string, apiCall: (url: string, options?: RequestInit) => Promise<Response>) {
  const [stats, setStats] = useState<Record<number, UserStats>>({});
  // Which users have a live request right now. Kept separate from the stored status:
  // a row left as 'pending' by an earlier failed run must not disable the button forever.
  const [inFlight, setInFlight] = useState<Record<number, boolean>>({});
  const streamsRef = useRef<Map<number, OpenStream>>(new Map());

  const closeStream = useCallback((userId: number) => {
    const stream = streamsRef.current.get(userId);
    if (stream) {
      clearTimeout(stream.timeoutId);
      stream.source.close();
      streamsRef.current.delete(userId);
    }
    setInFlight((previous) => {
      if (!previous[userId]) return previous;
      const next = { ...previous };
      delete next[userId];
      return next;
    });
  }, []);

  // Close every stream when the component goes away, so navigating off the page
  // does not leave connections open against the API.
  useEffect(() => {
    const streams = streamsRef.current;
    return () => {
      streams.forEach((stream) => {
        clearTimeout(stream.timeoutId);
        stream.source.close();
      });
      streams.clear();
    };
  }, []);

  /** Loads all previously calculated stats in one request, for the initial render. */
  const loadAllStats = useCallback(async () => {
    try {
      const response = await apiCall(`${apiUrl}/api/${backendName}/users/stats`);
      if (!response.ok) return;

      const rows: UserStats[] = await response.json();
      setStats(Object.fromEntries(rows.map((row) => [row.user_id, row])));
    } catch (error) {
      console.error('Error loading user stats:', error);
    }
  }, [apiUrl, backendName, apiCall]);

  const calculateStats = useCallback(
    (userId: number) => {
      // A second press while one is in flight restarts cleanly.
      closeStream(userId);

      setInFlight((previous) => ({ ...previous, [userId]: true }));
      setStats((previous) => ({
        ...previous,
        [userId]: { ...previous[userId], user_id: userId, status: 'pending' } as UserStats,
      }));

      const source = new EventSource(`${apiUrl}/api/${backendName}/users/${userId}/stats/stream`, {
        withCredentials: true,
      });

      const timeoutId = setTimeout(() => {
        console.error(`Timed out waiting for stats of user ${userId}`);
        setStats((previous) => ({
          ...previous,
          [userId]: {
            ...previous[userId],
            user_id: userId,
            status: 'failed',
            error_message: 'Timed out waiting for the calculation',
          } as UserStats,
        }));
        closeStream(userId);
      }, RESULT_TIMEOUT_MS);

      const stream: OpenStream = { source, timeoutId, requested: false };
      streamsRef.current.set(userId, stream);

      // Publish the Kafka request only once the stream is established.
      source.onopen = async () => {
        if (stream.requested) return;
        stream.requested = true;

        try {
          const response = await apiCall(`${apiUrl}/api/${backendName}/users/${userId}/stats/recalculate`, {
            method: 'POST',
          });

          if (!response.ok) {
            const body = await response.json().catch(() => null);
            throw new Error(body?.error ?? 'Failed to queue calculation');
          }
        } catch (error) {
          console.error('Error requesting stats calculation:', error);
          setStats((previous) => ({
            ...previous,
            [userId]: {
              ...previous[userId],
              user_id: userId,
              status: 'failed',
              error_message: error instanceof Error ? error.message : 'Failed to queue calculation',
            } as UserStats,
          }));
          closeStream(userId);
        }
      };

      // Last known value, replayed on connect. Render it, but keep waiting.
      source.addEventListener('snapshot', (event) => {
        const snapshot: UserStats = JSON.parse((event as MessageEvent).data);
        setStats((previous) => ({
          ...previous,
          // Never let a stale 'done' snapshot overwrite the pending state we are in.
          [userId]: { ...snapshot, status: previous[userId]?.status ?? snapshot.status },
        }));
      });

      // A finished calculation pushed through Kafka.
      source.addEventListener('stats', (event) => {
        const result: UserStats = JSON.parse((event as MessageEvent).data);
        setStats((previous) => ({ ...previous, [userId]: result }));

        if (result.status === 'done' || result.status === 'failed') {
          closeStream(userId);
        }
      });

      source.onerror = () => {
        // EventSource reconnects on its own; only give up once it is closed for good.
        if (source.readyState === EventSource.CLOSED) {
          console.error(`Stats stream closed for user ${userId}`);
          setStats((previous) => ({
            ...previous,
            [userId]: {
              ...previous[userId],
              user_id: userId,
              status: 'failed',
              error_message: 'Lost connection to the stats stream',
            } as UserStats,
          }));
          closeStream(userId);
        }
      };
    },
    [apiUrl, backendName, apiCall, closeStream]
  );

  return { stats, inFlight, loadAllStats, calculateStats };
}
