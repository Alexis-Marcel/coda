import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { User } from "oidc-client-ts";
import { api } from "../api/client";
import { configureAuth, restoreSession, signIn, signOut, toAuthUser, userManager, type AuthUser } from "../api/oidc";

interface AppContext {
  user: AuthUser | null;
  loading: boolean;
  accounts: boolean; // comptes configurés sur ce serveur
  signIn: (next?: string) => void;
  signOut: () => void;
}

const Ctx = createContext<AppContext | null>(null);

export function AppProvider({ children }: { children: ReactNode }) {
  const [oidcUser, setOidcUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [accounts, setAccounts] = useState(false);

  useEffect(() => {
    let cleanup = () => {};
    api
      .config()
      .then(async (cfg) => {
        configureAuth(cfg.oidcIssuer, cfg.oidcClientId);
        const m = userManager();
        setAccounts(Boolean(m));
        if (!m) return;
        setOidcUser(await restoreSession());
        const onLoaded = (u: User) => setOidcUser(u);
        const onUnloaded = () => setOidcUser(null);
        const onRenewError = () => void m.removeUser();
        m.events.addUserLoaded(onLoaded);
        m.events.addUserUnloaded(onUnloaded);
        m.events.addSilentRenewError(onRenewError);
        // Retour de connexion fini avant l'abonnement : on relit la session.
        const u = await m.getUser();
        if (u && !u.expired) setOidcUser(u);
        cleanup = () => {
          m.events.removeUserLoaded(onLoaded);
          m.events.removeUserUnloaded(onUnloaded);
          m.events.removeSilentRenewError(onRenewError);
        };
      })
      .catch(() => {})
      .finally(() => setLoading(false));
    return () => cleanup();
  }, []);

  const value = useMemo<AppContext>(
    () => ({
      user: toAuthUser(oidcUser),
      loading,
      accounts,
      signIn: (next) => void signIn(next),
      signOut: () => void signOut(),
    }),
    [oidcUser, loading, accounts],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useApp() {
  const c = useContext(Ctx);
  if (!c) throw new Error("useApp hors de <AppProvider>");
  return c;
}
