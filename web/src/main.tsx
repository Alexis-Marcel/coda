import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { api } from "./api/client";
import { configureAuth, userManager } from "./api/oidc";
import App from "./App";
import "./styles.css";

if (window.location.pathname === "/auth/silent") {
  // Iframe de connexion silencieuse : on rend la main au parent, rien à afficher.
  void api.config().then((cfg) => {
    configureAuth(cfg.auth.oidc?.issuer ?? "", cfg.auth.oidc?.clientId ?? "", cfg.auth.oidc?.metadata);
    void userManager()?.signinSilentCallback().catch(() => {});
  });
} else {
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}
