import React from 'react';
import type { UserStats } from '../hooks/useUserStats';

const baseClasses = 'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap';

/**
 * The label shown next to a user's name once their character count is known.
 * Renders nothing until a calculation has been requested at least once.
 */
const StatsLabel: React.FC<{ stats?: UserStats }> = ({ stats }) => {
  if (!stats || stats.status === 'never') {
    return null;
  }

  if (stats.status === 'pending') {
    return (
      <span className={`${baseClasses} bg-amber-100 text-amber-800`}>
        <span className="h-2 w-2 animate-pulse rounded-full bg-amber-500" aria-hidden="true" />
        Calculating...
      </span>
    );
  }

  if (stats.status === 'failed') {
    return (
      <span className={`${baseClasses} bg-red-100 text-red-800`} title={stats.error_message}>
        Calculation failed
      </span>
    );
  }

  return (
    <span
      className={`${baseClasses} bg-emerald-100 text-emerald-800`}
      title={stats.calculated_at ? `Calculated at ${new Date(stats.calculated_at).toLocaleString()}` : undefined}
    >
      {stats.total_characters.toLocaleString()} chars
      <span className="text-emerald-600">·</span>
      {stats.posts_count} {stats.posts_count === 1 ? 'post' : 'posts'}
    </span>
  );
};

export default StatsLabel;
