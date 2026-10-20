import { useApp } from "../auth/AuthProvider";

/** Barre du portail dont le jeu fait partie : retour à son accueil, et au profil quand on est connecté. */
export function PortalBar() {
  const { portal, user } = useApp();
  if (!portal) return null;
  return (
    <nav className="portalbar" aria-label={portal.name}>
      <a href={portal.url} className="portalbar-home">
        {portal.icon && <img src={portal.icon} alt="" width={16} height={16} />}
        <span>{portal.name}</span>
      </a>
      {user && portal.profileUrl && (
        <a href={portal.profileUrl} className="portalbar-profile">
          Mon profil
        </a>
      )}
    </nav>
  );
}
