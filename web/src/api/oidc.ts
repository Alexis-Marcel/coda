import { InMemoryWebStorage, UserManager, WebStorageStateStore, type User } from "oidc-client-ts";

// Compte Eriphe (Authentik) : client public avec PKCE, jetons en mémoire
// seulement. Au chargement et au renouvellement, une iframe cachée
// (prompt=none) demande au fournisseur : déjà connecté sur eriphe.com, on l'est
// ici sans clic. La configuration vient du serveur (/api/config).

let manager: UserManager | null = null;

export function configureAuth(issuer: string, clientId: string) {
  if (!issuer || !clientId || manager) return;
  manager = new UserManager({
    authority: issuer,
    client_id: clientId,
    redirect_uri: `${window.location.origin}/auth/callback`,
    silent_redirect_uri: `${window.location.origin}/auth/silent`,
    silentRequestTimeoutInSeconds: 5,
    post_logout_redirect_uri: `${window.location.origin}/`,
    scope: "openid email profile",
    automaticSilentRenew: true,
    userStore: new WebStorageStateStore({ store: new InMemoryWebStorage() }),
  });
}

export function userManager(): UserManager | null {
  return manager;
}

export interface AuthUser {
  id: string;
  name: string;
}

export function toAuthUser(u: User | null): AuthUser | null {
  if (!u || u.expired) return null;
  const p = u.profile;
  return { id: p.sub, name: (p.name || p.preferred_username || "").trim() };
}

let restoring: Promise<User | null> | null = null;

/** Session au chargement, sans jamais échouer : sans session chez le fournisseur, on reste invité. */
export function restoreSession(): Promise<User | null> {
  restoring ??= (async () => {
    if (!manager) return null;
    const stored = await manager.getUser();
    if (stored && !stored.expired) return stored;
    if (window.top !== window.self || window.location.pathname.startsWith("/auth/")) return null;
    try {
      return await manager.signinSilent();
    } catch {
      return null;
    }
  })();
  return restoring;
}

export async function accessToken(): Promise<string | null> {
  if (!manager) return null;
  await restoreSession();
  const u = await manager.getUser();
  return u && !u.expired ? u.access_token : null;
}

let signingOut = false;

export function signIn(next = window.location.pathname) {
  if (!manager || signingOut) return Promise.resolve();
  return manager.signinRedirect({ state: { next } });
}

export function signOut() {
  signingOut = true;
  return manager?.signoutRedirect();
}
