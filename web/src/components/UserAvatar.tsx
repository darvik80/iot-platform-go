import { getToken } from '../api/client';
import type { User } from '../types';

const COLORS = ['#3d9cf0', '#3ecf8e', '#e6a23c', '#f07178', '#ba68c8', '#4fc3f7', '#81c784', '#ffb74d'];

function colorForId(id: number): string {
  return COLORS[id % COLORS.length];
}

function initials(user: User): string {
  const name = user.display_name || user.login || user.email || '?';
  const parts = name.split(/[\s._-]+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return (parts[0]?.[0] || '?').toUpperCase();
}

interface Props {
  user: User;
  size?: number;
  className?: string;
  onClick?: () => void;
}

export function UserAvatar({ user, size = 32, className, onClick }: Props) {
  const s: React.CSSProperties = {
    width: size,
    height: size,
    borderRadius: '50%',
    flexShrink: 0,
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: size * 0.42,
    fontWeight: 600,
    color: '#fff',
    background: colorForId(user.id),
    overflow: 'hidden',
    cursor: onClick ? 'pointer' : undefined,
    border: '2px solid var(--border)',
  };

  if (user.avatar_url) {
    const token = getToken();
    if (!token) {
      // No token yet — fall back to initials until auth is ready
      return (
        <span className={className} style={s} onClick={onClick} role={onClick ? 'button' : undefined}>
          {initials(user)}
        </span>
      );
    }
    const params = new URLSearchParams();
    params.set('token', token);
    if (user.updated_at) params.set('v', String(user.updated_at));
    const src = `${user.avatar_url}?${params.toString()}`;
    return (
      <img
        src={src}
        alt=""
        className={className}
        style={{ ...s, objectFit: 'cover' }}
        onClick={onClick}
      />
    );
  }

  return (
    <span className={className} style={s} onClick={onClick} role={onClick ? 'button' : undefined}>
      {initials(user)}
    </span>
  );
}
