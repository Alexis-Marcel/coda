# Coda

Le jeu de déduction **Coda** (Da Vinci Code) en ligne, de 2 à 4 joueurs. Chacun a une rangée de tuiles numérotées, cachées et
rangées dans l'ordre ; à tour de rôle on devine celles des autres une à une, et le dernier à garder une tuile cachée gagne.

En ligne : [coda.eriphe.com](https://coda.eriphe.com). On joue en invité, ou avec son Compte Eriphe pour garder ses parties.

## Règles retenues

- 24 tuiles : 0 à 11 en noir et en blanc (sans joker). 4 tuiles chacun à 2 ou 3 joueurs, 3 à 4 joueurs.
- Une rangée est triée par valeur, la noire avant la blanche à valeur égale. Tout le monde voit la couleur des tuiles.
- À son tour : on pioche une tuile de la couleur de son choix (tant que la pioche n'est pas vide), puis on désigne une tuile
  cachée d'un adversaire en annonçant sa valeur.
  - Juste : elle est retournée ; on annonce encore, ou on s'arrête et la tuile piochée rejoint sa rangée, cachée.
  - Fausse : la tuile piochée rejoint sa rangée, retournée. Pioche vide : on retourne une de ses tuiles, au choix.
- Qui n'a plus de tuile cachée est éliminé ; le dernier en lice gagne.
- Temps par tour (30 s, 1 min, 2 min ou libre). Temps écoulé : le serveur pioche si besoin, une annonce manquante compte
  comme fausse, et après une annonce juste il s'arrête.
- L'hôte peut lancer avant que toutes les places soient prises (à partir de deux joueurs).

## Architecture

Un seul binaire Go sert l'API, les WebSockets et le front (Vite + React), sur le modèle d'Orapa :

```
cmd/server/        point d'entrée (variables d'environnement, arrêt propre)
internal/game/     moteur de règles pur : tuiles, pioche, annonces, éliminations
internal/server/   HTTP, WebSocket par partie, SQLite, comptes OIDC
web/               front : table, choix de la valeur, aide à la déduction, journal
deploy/nas/        stack Compose Manager du NAS
```

- **Moteur** sans I/O ni concurrence, l'heure et le hasard passés en paramètre ; testé à ~93 %, dont des parties entières
  jouées au hasard (rangées toujours triées, 24 tuiles en jeu, classement complet).
- **Vues par joueur** : chacun voit la valeur de ses tuiles et des tuiles retournées, seulement la couleur des autres ; la
  tuile piochée n'est connue que de celui qui l'a piochée. Chaque tuile porte un repère tiré au hasard, qui la désigne sans
  rien dire de sa valeur. Tout est dévoilé à la fin.
- **Aide à la déduction** (front) : sous chaque tuile cachée, les valeurs encore possibles d'après les tuiles vues, les
  annonces fausses et l'ordre de la rangée.
- **Parties** en mémoire, une par verrou, enregistrées entières dans SQLite à chaque coup. Au redémarrage, les parties
  reprennent et le temps d'arrêt n'est décompté à personne.

## Développement

```sh
go test ./...                                   # moteur + serveur
go run ./cmd/server                             # :8080, données dans ./data
cd web && npm install && npm run dev            # :5173, /api et /ws vers :8080
```

Variables : `ADDR` (`:8080`), `DATA_DIR` (`data`), `STATIC_DIR` (front compilé), `ALLOWED_ORIGINS` (origines
WebSocket en plus du même hôte).

### Comptes (facultatifs)

On joue toujours en invité ; un compte sert à garder ses parties. La méthode se choisit à l'hébergement, par
[gamekit/auth](https://github.com/alexis-marcel/gamekit) :

| Variable | Rôle | Défaut |
|---|---|---|
| `AUTH_MODE` | `none` (pas de comptes), `oidc` (Authentik, Keycloak, Authelia, Google…) ou `proxy` (reverse proxy qui authentifie déjà) | `oidc` si `OIDC_ISSUER` est donné |
| `GUEST_PLAY` | `false` : un compte est exigé pour créer ou rejoindre une partie | `true` |
| `ACCOUNT_NAME` | nom du fournisseur dans l'interface (« Se connecter avec … ») | « Se connecter » |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_AUDIENCE` | fournisseur OIDC ; client public avec PKCE, redirections `/auth/callback` et `/auth/silent` | client : `coda`, audience : le client |
| `TRUSTED_PROXIES`, `PROXY_USER_HEADER`, `PROXY_NAME_HEADER`, `PROXY_LOGIN_URL`, `PROXY_LOGOUT_URL` | mode `proxy` : voir [gamekit](https://github.com/alexis-marcel/gamekit#auth--comptes-au-choix-de-celui-qui-héberge) | `Remote-User`, `Remote-Name` |

## Déploiement

À chaque push sur `main`, l'action « Image Docker » publie `ghcr.io/alexis-marcel/coda` (amd64 et arm64).
`deploy/nas/deploy.sh` (ou « Update stack » dans Unraid) récupère la nouvelle image et relance la stack.
