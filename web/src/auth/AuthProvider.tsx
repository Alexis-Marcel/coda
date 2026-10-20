import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { User } from "oidc-client-ts";
import { api } from "../api/client";
import { configureAuth, restoreSession, signIn, signOut, toAuthUser, userManager, type AuthUser } from "../api/oidc";

interface AppContext {
  user: AuthUser | null;
  loading: boolean;
  accounts: boolean; // comptes configurés sur ce serveur
  accountName: string; // nom du fournisseur de comptes, vide si l'hébergeur n'en donne pas
  guestPlay: boolean; // jouer sans compte est permis
  canSignOut: boolean; // un bouton de déconnexion a un sens (mode proxy : seulement si le proxy en donne une page)
  signIn: (next?: string) => void;
  signOut: () => void;
}

const Ctx = createContext<AppContext | null>(null);

export function AppProvider({ children }: { children: ReactNode }) {
  const [oidcUser, setOidcUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [accounts, setAccounts] = useState(false);
  const [accountName, setAccountName] = useState("");
  const [guestPlay, setGuestPlay] = useState(true);
  // Mode proxy : le reverse proxy connecte la personne, le serveur dit qui c'est.
  const [proxy, setProxy] = useState<{ user: AuthUser | null; loginUrl?: string; logoutUrl?: string } | null>(null);

  useEffect(() => {
    let cleanup = () => {};
    api
      .config()
      .then(async (cfg) => {
        setAccountName(cfg.auth.accountName ?? "");
        setGuestPlay(cfg.auth.guestPlay);
        if (cfg.auth.mode === "proxy") {
          const p = { user: cfg.user, ...cfg.auth.proxy };
          setProxy(p);
          setAccounts(Boolean(p.user || p.loginUrl));
          return;
        }
        configureAuth(cfg.auth.oidc?.issuer ?? "", cfg.auth.oidc?.clientId ?? "");
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
      user: proxy ? proxy.user : toAuthUser(oidcUser),
      loading,
      accounts,
      accountName,
      guestPlay,
      canSignOut: !proxy || Boolean(proxy.logoutUrl),
      signIn: (next) => {
        if (!proxy) void signIn(next);
        else if (proxy.loginUrl) window.location.href = proxy.loginUrl;
      },
      signOut: () => {
        if (!proxy) void signOut();
        else if (proxy.logoutUrl) window.location.href = proxy.logoutUrl;
      },
    }),
    [oidcUser, proxy, loading, accounts, accountName, guestPlay],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useApp() {
  const c = useContext(Ctx);
  if (!c) throw new Error("useApp hors de <AppProvider>");
  return c;
}
