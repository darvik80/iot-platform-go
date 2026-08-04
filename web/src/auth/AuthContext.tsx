import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { api, getToken, onUnauthorized, setToken } from '../api/client';
import { platformWS } from '../api/ws';
import type { AuthConfig, Settings, User } from '../types';

interface AuthState {
  token: string;
  user: User | null;
  settings: Settings | null;
  authConfig: AuthConfig | null;
  ready: boolean;
  login: (identity: string, password: string) => Promise<void>;
  logout: () => void;
  refreshMe: () => Promise<void>;
  setSession: (token: string) => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setTok] = useState(getToken());
  const [user, setUser] = useState<User | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [authConfig, setAuthConfig] = useState<AuthConfig | null>(null);
  const [ready, setReady] = useState(false);

  const logout = useCallback(() => {
    setToken('');
    setTok('');
    setUser(null);
    platformWS.close();
  }, []);

  const refreshMe = useCallback(async () => {
    const me = await api<User>('/me');
    setUser(me);
    try {
      setSettings(await api<Settings>('/settings'));
    } catch {
      /* ignore */
    }
  }, []);

  const setSession = useCallback(
    async (t: string) => {
      setToken(t);
      setTok(t);
      platformWS.connect(t);
      await refreshMe();
    },
    [refreshMe],
  );

  const login = useCallback(
    async (identity: string, password: string) => {
      const data = await api<{ token: string; user: User }>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email: identity, password }),
      });
      await setSession(data.token);
    },
    [setSession],
  );

  useEffect(() => {
    return onUnauthorized(() => logout());
  }, [logout]);

  useEffect(() => {
    (async () => {
      try {
        setAuthConfig(await fetch('/api/v1/auth/config').then((r) => r.json()));
      } catch {
        setAuthConfig({});
      }
      const hash = location.hash || '';
      if (hash.startsWith('#auth=')) {
        const t = decodeURIComponent(hash.slice(6));
        history.replaceState(null, '', location.pathname);
        try {
          await setSession(t);
        } catch {
          logout();
        }
        setReady(true);
        return;
      }
      if (token) {
        try {
          platformWS.connect(token);
          await refreshMe();
        } catch {
          logout();
        }
      }
      setReady(true);
    })();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const value = useMemo(
    () => ({ token, user, settings, authConfig, ready, login, logout, refreshMe, setSession }),
    [token, user, settings, authConfig, ready, login, logout, refreshMe, setSession],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth outside provider');
  return ctx;
}
