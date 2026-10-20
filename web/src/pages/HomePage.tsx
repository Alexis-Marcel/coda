import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, guestName, saveGuestName, saveSeatToken, seatTokens } from "../api/client";
import type { CurrentGame, Me, OpenGame, TileView } from "../api/types";
import { useApp } from "../auth/AuthProvider";
import { Header } from "../components/Header";
import { Tile } from "../components/Tile";

// La rangée d'un adversaire, telle qu'on la voit en cours de partie : deux
// tuiles retournées encadrent les autres, la dernière vient d'être trouvée.
const DEMO: (TileView & { hint?: string })[] = [
  { id: 0, color: 0, num: 1, revealed: true },
  { id: 1, color: 1, revealed: false, hint: "1–5" },
  { id: 2, color: 0, revealed: false, hint: "2–6" },
  { id: 3, color: 1, num: 6, revealed: true },
  { id: 4, color: 0, num: 9, revealed: true },
];

export const TURN_TIMES = [
  { s: 30, label: "30 s" },
  { s: 60, label: "1 min" },
  { s: 120, label: "2 min" },
  { s: 0, label: "Libre" },
];

export function turnLabel(ms: number) {
  return TURN_TIMES.find((t) => t.s * 1000 === ms)?.label ?? `${Math.round(ms / 1000)} s`;
}

function CreateForm() {
  const { user, loading } = useApp();
  const navigate = useNavigate();
  const [name, setName] = useState(guestName);
  const [seats, setSeats] = useState(2);
  const [turnTime, setTurnTime] = useState(60);
  const [isPublic, setPublic] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (ev: FormEvent) => {
    ev.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (!user) saveGuestName(name.trim());
      const r = await api.create(name.trim(), seats, turnTime, isPublic);
      saveSeatToken(r.id, r.token);
      navigate(`/g/${r.id}`);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };

  return (
    <form className="panel" onSubmit={submit}>
      <h2>Nouvelle partie</h2>
      {/* Le temps de savoir si l'on est connecté, pas de champ « invité ». */}
      {!user && !loading && (
        <label className="field">
          <span>Ton nom</span>
          <input value={name} onChange={(e) => setName(e.target.value)} maxLength={24} placeholder="Invité" autoComplete="nickname" />
        </label>
      )}
      <fieldset className="field">
        <legend>Joueurs</legend>
        <div className="segmented">
          {[2, 3, 4].map((n) => (
            <label key={n} className={seats === n ? "is-on" : ""}>
              <input type="radio" name="seats" checked={seats === n} onChange={() => setSeats(n)} />
              <strong>{n}</strong>
              <small>{n === 4 ? "3 tuiles" : "4 tuiles"}</small>
            </label>
          ))}
        </div>
      </fieldset>
      <fieldset className="field">
        <legend>Temps par tour</legend>
        <div className="segmented">
          {TURN_TIMES.map((t) => (
            <label key={t.s} className={turnTime === t.s ? "is-on" : ""}>
              <input type="radio" name="turn" checked={turnTime === t.s} onChange={() => setTurnTime(t.s)} />
              <strong>{t.label}</strong>
            </label>
          ))}
        </div>
      </fieldset>
      <label className="check">
        <input type="checkbox" checked={isPublic} onChange={(e) => setPublic(e.target.checked)} />
        <span>Visible dans les parties ouvertes (sinon, seulement par le lien)</span>
      </label>
      {error && <p className="error">{error}</p>}
      <button className="btn btn-primary" disabled={busy || loading}>
        Créer la partie
      </button>
    </form>
  );
}

// Parties pas finies de ce navigateur (ou du compte), pour y revenir après avoir fermé l'onglet.
function CurrentGames() {
  const { user } = useApp();
  const [games, setGames] = useState<CurrentGame[]>([]);
  useEffect(() => {
    let stop = false;
    const load = () => api.current(seatTokens()).then((g) => !stop && setGames(g), () => {});
    void load();
    const t = setInterval(load, 10000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [user]);
  if (games.length === 0) return null;
  return (
    <section className="panel current">
      <h2>Tes parties en cours</h2>
      <ul className="rows">
        {games.map((g) => (
          <li key={g.id}>
            <span className="row-main">{g.opponents.length ? `avec ${g.opponents.join(", ")}` : "en attente des autres joueurs"}</span>
            <span className={g.yourMove ? "turn-mine" : "muted"}>{currentStatus(g)}</span>
            <Link className={`btn btn-small ${g.yourMove ? "btn-primary" : ""}`} to={`/g/${g.id}`}>
              Reprendre
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function currentStatus(g: CurrentGame): string {
  if (g.phase === "waiting") return `${g.players}/${g.seats} joueurs`;
  if (g.out) return "éliminé, la partie continue";
  return g.yourMove ? "à toi de jouer" : `à ${g.turn}`;
}

function OpenGames() {
  const [games, setGames] = useState<OpenGame[] | null>(null);
  useEffect(() => {
    let stop = false;
    const load = () => api.open().then((g) => !stop && setGames(g), () => {});
    void load();
    const t = setInterval(load, 5000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  return (
    <section className="panel">
      <h2>Parties ouvertes</h2>
      {games === null ? (
        <p className="muted">Chargement…</p>
      ) : games.length === 0 ? (
        <p className="muted">Aucune partie n'attend de joueur. Crée la tienne et envoie le lien.</p>
      ) : (
        <ul className="rows">
          {games.map((g) => (
            <li key={g.id}>
              <span className="row-main">{g.host}</span>
              <span className="muted">
                {g.players}/{g.seats} joueurs, {turnLabel(g.turnMs)}
              </span>
              <Link className="btn btn-small" to={`/g/${g.id}`}>
                Rejoindre
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function MyGames() {
  const { user } = useApp();
  const [me, setMe] = useState<Me | null>(null);
  useEffect(() => {
    if (user) api.me().then(setMe, () => setMe(null));
  }, [user]);
  if (!user || !me) return null;
  return (
    <section className="panel">
      <h2>Tes parties</h2>
      {me.stats.played === 0 ? (
        <p className="muted">Pas encore de partie terminée.</p>
      ) : (
        <>
          <p className="stats">
            {me.stats.won} victoire{me.stats.won > 1 ? "s" : ""} en {me.stats.played} partie{me.stats.played > 1 ? "s" : ""}
          </p>
          <ul className="rows">
            {me.games.slice(0, 8).map((g) => (
              <li key={g.id}>
                <span className={`outcome ${g.rank === 1 ? "outcome-win" : "outcome-loss"}`}>
                  {g.rank === 1 ? "Victoire" : `${g.rank}e sur ${g.players}`}
                </span>
                <span className="row-main">contre {g.opponents.join(", ")}</span>
                <Link className="btn btn-small btn-quiet" to={`/g/${g.id}`}>
                  Revoir
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

function Rules() {
  return (
    <section className="panel rules">
      <h2>Comment on joue</h2>
      <ol>
        <li>
          24 tuiles : 0 à 11 en noir et en blanc. Chacun en reçoit quatre (trois à quatre joueurs), rangées de la plus petite à la plus grande,
          la noire avant la blanche à valeur égale. Tout le monde voit leur couleur, seul toi vois leur valeur.
        </li>
        <li>À ton tour, pioche une tuile, noire ou blanche : elle reste devant toi, cachée.</li>
        <li>
          Désigne une tuile cachée d'un adversaire et annonce sa valeur. Juste, elle est retournée : annonce encore, ou arrête-toi et range ta
          tuile piochée sans la montrer.
        </li>
        <li>Fausse, ta tuile piochée rejoint ta rangée retournée. Si la pioche est vide, tu retournes une de tes tuiles.</li>
        <li>Qui n'a plus de tuile cachée est éliminé. Le dernier en lice gagne.</li>
      </ol>
      <p className="muted small">Sous chaque tuile, Coda affiche les valeurs encore possibles d'après ce que tu as vu.</p>
    </section>
  );
}

export function HomePage() {
  return (
    <>
      <Header />
      <main className="page home">
        <section className="hero">
          <div className="hero-text">
            <h1>Coda</h1>
            <p className="lede">
              Chacun cache une rangée de tuiles numérotées, dans l'ordre. Devine celles des autres une à une ; le dernier à garder une tuile
              cachée gagne.
            </p>
          </div>
          <div className="hero-row" aria-label="Exemple : la rangée d'un adversaire, une noire 1, deux tuiles cachées, une blanche 6, une noire 9">
            {DEMO.map((t, i) => (
              <Tile key={t.id} tile={t} size="l" hint={t.hint} fresh={i === 4} />
            ))}
          </div>
        </section>
        <div className="home-grid">
          <div className="home-col">
            <CurrentGames />
            <CreateForm />
            <MyGames />
          </div>
          <div className="home-col">
            <OpenGames />
            <Rules />
          </div>
        </div>
      </main>
    </>
  );
}
