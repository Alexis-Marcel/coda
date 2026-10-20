import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { User } from "oidc-client-ts";
import { api } from "../api/client";
import type { PortalInfo } from "../api/types";
import { configureAuth, restoreSession, signIn, signOut, toAuthUser, userManager, type AuthUser } from "../api/oidc";

interface AppContext {
  user: AuthUser | null;
  loading: boolean;
  accounts: boolean; // comptes configurés sur ce serveur
  accountName: string; // nom du fournisseur de comptes, vide si l'hébergeur n'en donne pas
  guestPlay: boolean; // jouer sans compte est permis
  accountUrl: string; // page du compte (mode local), vide sinon
  portal: PortalInfo | null; // portail dont le jeu fait partie, null s'il vit seul
  canSignOut: boolean; // un bouton de déconnexion a un sens (mode proxy : seulement si le proxy en donne une page)
  signIn: (next?: string) => void;
  signOut: () => void;
}

// Session tenue par le serveur (modes proxy et local) : qui est connecté, et les pages pour entrer ou sortir.
interface ServerSession {
  user: AuthUser | null;
  loginUrl?: string;
  logoutUrl?: string;
  accountUrl?: string;
  local?: boolean;
}

const Ctx = createContext<AppContext | null>(null);

export function AppProvider({ children }: { children: ReactNode }) {
  const [oidcUser, setOidcUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [accounts, setAccounts] = useState(false);
  const [accountName, setAccountName] = useState("");
  const [guestPlay, setGuestPlay] = useState(true);
  const [portal, setPortal] = useState<PortalInfo | null>(null);
  // Modes proxy et local : c'est le serveur qui sait qui est connecté (cookie ou en-tête du proxy).
  const [proxy, setProxy] = useState<ServerSession | null>(null);

  useEffect(() => {
    let cleanup = () => {};
    api
      .config()
      .then(async (cfg) => {
        setAccountName(cfg.auth.accountName ?? "");
        setGuestPlay(cfg.auth.guestPlay);
        setPortal(cfg.portal);
        if (cfg.auth.mode === "proxy" || cfg.auth.mode === "local") {
          const p: ServerSession = { user: cfg.user, ...(cfg.auth.local ? { ...cfg.auth.local, local: true } : cfg.auth.proxy) };
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
      accountUrl: proxy?.accountUrl ?? "",
      portal,
      canSignOut: !proxy || Boolean(proxy.logoutUrl),
      signIn: (next) => {
        if (!proxy) void signIn(next);
        else if (proxy.local) window.location.href = `${proxy.loginUrl}?next=${encodeURIComponent(next ?? window.location.pathname)}`;
        else if (proxy.loginUrl) window.location.href = proxy.loginUrl;
      },
      signOut: () => {
        if (!proxy) void signOut();
        else if (proxy.local) void fetch(proxy.logoutUrl!, { method: "POST" }).finally(() => window.location.reload());
        else if (proxy.logoutUrl) window.location.href = proxy.logoutUrl;
      },
    }),
    [oidcUser, proxy, loading, accounts, accountName, guestPlay, portal],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useApp() {
  const c = useContext(Ctx);
  if (!c) throw new Error("useApp hors de <AppProvider>");
  return c;
}
