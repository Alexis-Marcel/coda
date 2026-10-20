import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { configureAuth, userManager } from "../api/oidc";

export function AuthCallback() {
  const navigate = useNavigate();
  const [failed, setFailed] = useState(false);
  const once = useRef(false);
  useEffect(() => {
    if (once.current) return;
    once.current = true;
    (async () => {
      const cfg = await api.config();
      configureAuth(cfg.auth.oidc?.issuer ?? "", cfg.auth.oidc?.clientId ?? "");
      const u = await userManager()!.signinRedirectCallback();
      const next = (u.state as { next?: string } | undefined)?.next ?? "/";
      navigate(next.startsWith("/") ? next : "/", { replace: true });
    })().catch(() => setFailed(true));
  }, [navigate]);
  return (
    <main className="page page-narrow">
      <p className="muted">{failed ? "La connexion n'a pas abouti." : "Connexion…"}</p>
      {failed && <a href="/">Revenir à l'accueil</a>}
    </main>
  );
}
