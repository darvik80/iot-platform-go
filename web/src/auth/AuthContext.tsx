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
import type { AccountMembership, AuthConfig, MeResponse, Settings, User } from '../types';

interface AuthState {
  token: string;
  user: User | null;
  settings: Settings | null;
  authConfig: AuthConfig | null;
  ready: boolean;
  memberships: AccountMembership[];
  activeAccountId: number;
  activeMembership: AccountMembership | null;
  setupNeeded: boolean;
  login: (identity: string, password: string) => Promise<void>;
  logout: () => void;
  refreshMe: () => Promise<void>;
  setSession: (token: string) => Promise<void>;
  switchAccount: (accountId: number) => Promise<void>;
  markSetupComplete: () => void;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setTok] = useState(getToken());
  const [user, setUser] = useState<User | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [authConfig, setAuthConfig] = useState<AuthConfig | null>(null);
  const [ready, setReady] = useState(false);
  const [memberships, setMemberships] = useState<AccountMembership[]>([]);
  const [activeAccountId, setActiveAccountId] = useState(0);
  const [setupNeeded, setSetupNeeded] = useState(false);

  const logout = useCallback(() => {
    setToken('');
    setTok('');
    setUser(null);
    setMemberships([]);
    setActiveAccountId(0);
    platformWS.close();
  }, []);

  const refreshMe = useCallback(async () => {
    const me = await api<MeResponse>('/me');
    setUser(me.user);
    setMemberships(me.memberships || []);
    setActiveAccountId(me.active_account_id || 0);
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

  const switchAccount = useCallback(
    async (accountId: number) => {
      const data = await api<{ token: string }>('/auth/switch-account', {
        method: 'POST',
        body: JSON.stringify({ account_id: accountId }),
      });
      await setSession(data.token);
    },
    [setSession],
  );

  const markSetupComplete = useCallback(() => {
    setSetupNeeded(false);
  }, []);

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
      
      // Check if setup wizard is needed
      try {
        const setupState = await fetch('/api/v1/setup/state').then((r) => r.json());
        if (!setupState.completed) {
          setSetupNeeded(true);
          setReady(true);
          return;
        }
      } catch {
        // Setup endpoint not available, continue normally
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

  const activeMembership = useMemo(
    () => memberships.find((m) => m.account_id === activeAccountId) ?? null,
    [memberships, activeAccountId],
  );

  const value = useMemo(
    () => ({ token, user, settings, authConfig, ready, memberships, activeAccountId, activeMembership, setupNeeded, login, logout, refreshMe, setSession, switchAccount, markSetupComplete }),
    [token, user, settings, authConfig, ready, memberships, activeAccountId, activeMembership, setupNeeded, login, logout, refreshMe, setSession, switchAccount, markSetupComplete],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth outside provider');
  return ctx;
}
