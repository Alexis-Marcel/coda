import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, guestName, saveGuestName, saveSeatToken, seatToken } from "../api/client";
import type { Action, Color, GameView, TileView } from "../api/types";
import { useGame, type GameSocket } from "../api/useGame";
import { useApp } from "../auth/AuthProvider";
import { Header } from "../components/Header";
import { Tile } from "../components/Tile";
import { TurnTimer } from "../components/TurnTimer";
import { NUMS, candidates, colorName, seen, summary } from "../lib/deduce";
import { turnLabel } from "./HomePage";

export function GamePage({ id }: { id: string }) {
  const [token, setToken] = useState(() => seatToken(id));
  const sock = useGame(id, token);
  const navigate = useNavigate();
  const s = sock.state;

  // Revanche acceptée pendant qu'on regarde : la place suit dans la nouvelle
  // partie. (Revoir une vieille partie ne renvoie pas vers sa revanche.)
  const firstNext = useRef<string | null>(null);
  useEffect(() => {
    if (!s) return;
    firstNext.current ??= s.next ?? "";
    if (s.next && s.next !== firstNext.current && token) {
      saveSeatToken(s.next, token);
      navigate(`/g/${s.next}`);
    }
  }, [s, token, navigate]);

  if (sock.notFound) {
    return (
      <>
        <Header />
        <main className="page page-narrow">
          <h1>Partie introuvable</h1>
          <p className="muted">Le lien est peut-être incomplet.</p>
          <Link className="btn" to="/">
            Revenir à l'accueil
          </Link>
        </main>
      </>
    );
  }
  if (!s) {
    return (
      <>
        <Header />
        <main className="page page-narrow">
          <p className="muted">{sock.conn === "closed" ? "Connexion au serveur…" : "Chargement de la partie…"}</p>
        </main>
      </>
    );
  }

  return (
    <>
      <Header>{sock.conn !== "open" && <span className="conn">Reconnexion…</span>}</Header>
      <main className="page game">
        {s.phase === "waiting" && <Waiting s={s} sock={sock} onJoined={setToken} />}
        {s.phase === "canceled" && (
          <div className="page-narrow">
            <h1>Partie annulée</h1>
            <Link className="btn" to="/">
              Revenir à l'accueil
            </Link>
          </div>
        )}
        {(s.phase === "playing" || s.phase === "finished") && <Table s={s} sock={sock} />}
      </main>
    </>
  );
}

// ---- salle d'attente

function Waiting({ s, sock, onJoined }: { s: GameView; sock: GameSocket; onJoined: (t: string) => void }) {
  const { user, accounts, signIn } = useApp();
  const [name, setName] = useState(guestName);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const url = window.location.href;
  const seated = s.you >= 0;

  const join = async (ev: FormEvent) => {
    ev.preventDefault();
    setError(null);
    try {
      if (!user) saveGuestName(name.trim());
      const r = await api.join(s.id, name.trim());
      saveSeatToken(s.id, r.token);
      onJoined(r.token);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const seats = Array.from({ length: s.seats }, (_, i) => s.players[i]);
  return (
    <div className="page-narrow waiting">
      <h1>{seated ? "En attente des joueurs" : `${s.players[0].name} t'invite à jouer`}</h1>
      <p className="lede">
        {s.seats} joueurs, {turnLabel(s.turnMs) === "Libre" ? "sans limite de temps par tour" : `${turnLabel(s.turnMs)} par tour`}.
        {seated && " La partie commence quand toutes les places sont prises."}
      </p>
      <ol className="seats">
        {seats.map((p, i) => (
          <li key={i} className={p ? "" : "is-free"}>
            {p ? (
              <>
                <span className={`dot ${p.online ? "is-on" : ""}`} aria-label={p.online ? "en ligne" : "absent"} />
                <span>
                  {p.name}
                  {i === s.you && <span className="muted"> (toi)</span>}
                </span>
                {i === 0 && <span className="muted small">hôte</span>}
              </>
            ) : (
              <span className="muted">Place libre</span>
            )}
          </li>
        ))}
      </ol>
      {seated && (
        <div className="share">
          <input readOnly value={url} onFocus={(e) => e.target.select()} aria-label="Lien de la partie" />
          <button className="btn" onClick={() => void navigator.clipboard?.writeText(url).then(() => setCopied(true))}>
            {copied ? "Lien copié" : "Copier le lien"}
          </button>
        </div>
      )}
      {s.you === 0 && (
        <div className="actions">
          <button className="btn btn-primary" disabled={s.players.length < 2} onClick={() => sock.send({ t: "start" })}>
            {s.players.length < 2 ? "Lancer dès qu'un joueur arrive" : `Lancer à ${s.players.length}`}
          </button>
          <button className="btn btn-quiet" onClick={() => sock.send({ t: "cancel" })}>
            Annuler la partie
          </button>
        </div>
      )}
      {!seated && (
        <form onSubmit={join}>
          {user ? (
            <p>
              Tu joues en tant que <strong>{user.name}</strong>.
            </p>
          ) : (
            <>
              <label className="field">
                <span>Ton nom</span>
                <input value={name} onChange={(e) => setName(e.target.value)} maxLength={24} placeholder="Invité" autoComplete="nickname" />
              </label>
              {accounts && (
                <p className="muted">
                  Ou{" "}
                  <button type="button" className="link" onClick={() => signIn()}>
                    connecte-toi avec ton compte Eriphe
                  </button>{" "}
                  pour garder tes parties.
                </p>
              )}
            </>
          )}
          <button className="btn btn-primary">Prendre une place</button>
        </form>
      )}
      {(error || sock.error) && <p className="error">{error ?? sock.error}</p>}
    </div>
  );
}

// ---- la table

interface Pick {
  seat: number;
  index: number;
}

function Table({ s, sock }: { s: GameView; sock: GameSocket }) {
  const [pick, setPick] = useState<Pick | null>(null);
  const [help, setHelp] = useState(() => {
    try {
      return localStorage.getItem("coda:help") !== "0";
    } catch {
      return true;
    }
  });
  const toggleHelp = () => {
    setHelp(!help);
    try {
      localStorage.setItem("coda:help", help ? "0" : "1");
    } catch {
      /* rien */
    }
  };

  const playing = s.phase === "playing";
  const myTurn = playing && s.turn === s.you;
  const canGuess = myTurn && (s.step === "guess" || s.step === "choose");
  const known = useMemo(() => seen(s), [s]);

  // Le choix d'une tuile ne survit pas à un changement de tour ou de rangée.
  const pickKey = pick && s.players[pick.seat]?.row[pick.index];
  useEffect(() => {
    if (!canGuess || !pickKey || pickKey.num !== undefined) setPick(null);
  }, [canGuess, pickKey]);

  // Tuile tout juste retournée : elle s'anime une fois.
  const last = s.actions[s.actions.length - 1];
  const freshId = last?.kind === "guess" ? (last.correct ? last.tile : last.penalty) : last?.kind === "reveal" ? last.tile : undefined;

  const order = seatOrder(s);
  const opponents = order.filter((i) => i !== s.you);
  const guess = (num: number) => {
    if (!pick) return;
    const t = s.players[pick.seat].row[pick.index];
    sock.send({ t: "guess", target: pick.seat, tile: t.id, num });
    setPick(null);
  };

  return (
    <div className="table">
      <div className="opponents">
        {opponents.map((seat) => (
          <Seat
            key={seat}
            s={s}
            seat={seat}
            receivedAt={sock.receivedAt}
            freshId={freshId}
            hints={help}
            known={known}
            pick={pick?.seat === seat ? pick.index : undefined}
            onPick={canGuess ? (index) => setPick(pick?.seat === seat && pick.index === index ? null : { seat, index }) : undefined}
          />
        ))}
      </div>

      <div className="center">
        {playing ? (
          <TurnPanel s={s} sock={sock} pick={pick} known={known} help={help} onGuess={guess} onCancel={() => setPick(null)} />
        ) : (
          <EndPanel s={s} sock={sock} />
        )}
      </div>

      {s.you >= 0 && (
        <Seat
          s={s}
          seat={s.you}
          receivedAt={sock.receivedAt}
          freshId={freshId}
          mine
          revealing={myTurn && s.step === "reveal"}
          onReveal={(id) => sock.send({ t: "reveal", tile: id })}
        />
      )}

      <aside className="side">
        <Journal s={s} />
        <div className="side-tools">
          <label className="check small">
            <input type="checkbox" checked={help} onChange={toggleHelp} />
            <span>Afficher les valeurs possibles</span>
          </label>
          {playing && s.you >= 0 && !s.players[s.you].out && (
            <button
              className="btn btn-small btn-quiet"
              onClick={() => window.confirm("Abandonner la partie ? Tu seras éliminé.") && sock.send({ t: "resign" })}
            >
              Abandonner
            </button>
          )}
        </div>
      </aside>
    </div>
  );
}

/** Ordre de jeu à partir de soi (ou du premier joueur pour un spectateur). */
function seatOrder(s: GameView) {
  const n = s.players.length;
  const from = Math.max(s.you, 0);
  return Array.from({ length: n }, (_, i) => (from + i) % n);
}

function Seat({
  s,
  seat,
  receivedAt,
  freshId,
  mine,
  hints,
  known,
  pick,
  onPick,
  revealing,
  onReveal,
}: {
  s: GameView;
  seat: number;
  receivedAt: number;
  freshId?: number;
  mine?: boolean;
  hints?: boolean;
  known?: ReturnType<typeof seen>;
  pick?: number;
  onPick?: (index: number) => void;
  revealing?: boolean;
  onReveal?: (id: number) => void;
}) {
  const p = s.players[seat];
  const turn = s.phase === "playing" && s.turn === seat;
  const status = p.out
    ? `éliminé, ${p.rank}e`
    : s.phase === "finished" && p.rank === 1
      ? "gagnant"
      : !p.online
        ? "absent"
        : "";
  return (
    <section className={`seat ${mine ? "seat-mine" : ""} ${turn ? "is-turn" : ""} ${p.out ? "is-out" : ""}`} aria-label={mine ? "Ta rangée" : `Rangée de ${p.name}`}>
      <header className="seat-head">
        <h2>
          {p.name}
          {mine && <span className="muted"> (toi)</span>}
        </h2>
        {status && <span className="seat-status">{status}</span>}
        {turn && <TurnTimer leftMs={s.turnLeftMs} totalMs={s.turnMs} receivedAt={receivedAt} />}
      </header>
      <div className="row">
        {p.row.map((t, i) => {
          const hidden = t.num === undefined;
          const clickable = (onPick && hidden) || (revealing && !t.revealed);
          const hint = hints && hidden && known ? summary(candidates(s, seat, i, known)) : undefined;
          return (
            <Tile
              key={t.id}
              tile={t}
              size={mine ? "l" : "m"}
              mine={mine}
              hint={hint}
              selected={pick === i}
              fresh={t.id === freshId}
              onClick={clickable ? () => (revealing ? onReveal?.(t.id) : onPick?.(i)) : undefined}
            />
          );
        })}
      </div>
    </section>
  );
}

function TurnPanel({
  s,
  sock,
  pick,
  known,
  help,
  onGuess,
  onCancel,
}: {
  s: GameView;
  sock: GameSocket;
  pick: Pick | null;
  known: ReturnType<typeof seen>;
  help: boolean;
  onGuess: (n: number) => void;
  onCancel: () => void;
}) {
  const myTurn = s.turn === s.you;
  const who = s.players[s.turn].name;
  const target = pick ? s.players[pick.seat].row[pick.index] : null;
  const possible = useMemo(() => (pick ? new Set(candidates(s, pick.seat, pick.index, known)) : null), [s, pick, known]);

  useEffect(() => {
    if (!pick) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onCancel();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [pick, onCancel]);

  let prompt: string;
  if (!myTurn) prompt = s.step === "draw" ? `${who} pioche.` : s.step === "reveal" ? `${who} retourne une de ses tuiles.` : `${who} cherche.`;
  else if (s.step === "draw") prompt = "Pioche une tuile.";
  else if (s.step === "guess") prompt = "Choisis une tuile cachée chez un adversaire et annonce sa valeur.";
  else if (s.step === "choose") prompt = "Juste ! Annonce encore, ou arrête-toi pour garder ta tuile cachée.";
  else prompt = "Raté, et la pioche est vide : retourne une de tes tuiles.";

  return (
    <div className="turn">
      <div className="pile" aria-label="Pioche">
        {([0, 1] as Color[]).map((c) => {
          const n = s.pile[c];
          const can = myTurn && s.step === "draw" && n > 0;
          return (
            <button
              key={c}
              className={`stack stack-${c === 0 ? "black" : "white"}`}
              disabled={!can}
              onClick={() => sock.send({ t: "draw", color: c })}
              aria-label={`Piocher une ${colorName(c)} (${n} restantes)`}
            >
              <span className="stack-count">{n}</span>
            </button>
          );
        })}
        {s.drawn && (
          <div className="drawn">
            <Tile tile={s.drawn} size="l" mine={myTurn} fresh />
            <span className="muted small">{myTurn ? "ta tuile piochée" : `piochée par ${who}`}</span>
          </div>
        )}
      </div>

      <p className={`prompt ${myTurn ? "is-mine" : ""}`} aria-live="polite">
        {prompt}
      </p>

      {myTurn && target && (
        <div className="picker" role="group" aria-label={`Valeur de la ${colorName(target.color)} de ${s.players[pick!.seat].name}`}>
          <p className="small">
            La {colorName(target.color)} de <strong>{s.players[pick!.seat].name}</strong> vaut…
          </p>
          <div className={`nums nums-${target.color === 0 ? "black" : "white"}`}>
            {NUMS.map((n) => (
              <button key={n} className={`num ${help && possible && !possible.has(n) ? "is-unlikely" : ""}`} onClick={() => onGuess(n)}>
                {n}
              </button>
            ))}
          </div>
          <button className="link small" onClick={onCancel}>
            Choisir une autre tuile
          </button>
        </div>
      )}

      {myTurn && s.step === "choose" && (
        <button className="btn btn-primary" onClick={() => sock.send({ t: "stop" })}>
          M'arrêter et garder ma tuile cachée
        </button>
      )}
      {sock.error && <p className="error">{sock.error}</p>}
    </div>
  );
}

function EndPanel({ s, sock }: { s: GameView; sock: GameSocket }) {
  const r = s.result!;
  const winner = s.players[r.winner];
  const you = s.you;
  const asked = you >= 0 && s.rematch[you];
  const waiting = s.players.filter((_, i) => !s.rematch[i]).map((p) => p.name);
  return (
    <div className="end">
      <h2 className="end-title">{r.winner === you ? "Tu as gagné" : `${winner.name} a gagné`}</h2>
      <p className="muted">{r.reason === "resign" ? "Les autres ont abandonné." : "Toutes les autres tuiles sont retournées."}</p>
      <div className="actions">
        {you >= 0 && !s.next && (
          <button className="btn btn-primary" disabled={asked} onClick={() => sock.send({ t: "rematch" })}>
            {asked ? "Revanche demandée" : "Rejouer avec les mêmes"}
          </button>
        )}
        {s.next && (
          <Link className="btn" to={`/g/${s.next}`}>
            Voir la revanche
          </Link>
        )}
        <Link className="btn btn-quiet" to="/">
          Accueil
        </Link>
      </div>
      {!s.next && s.rematch.some(Boolean) && <p className="muted small">En attente de : {waiting.join(", ")}.</p>}
    </div>
  );
}

// ---- journal

function tileText(t: TileView | undefined) {
  if (!t) return "";
  return `${colorName(t.color)}${t.num !== undefined ? ` ${t.num}` : ""}`;
}

function line(s: GameView, a: Action, tiles: Map<number, TileView>): { text: string; tone?: "ok" | "miss" | "out" } {
  const who = s.players[a.seat]?.name ?? "?";
  switch (a.kind) {
    case "draw":
      return { text: `${who} pioche une ${colorName(a.color)}.` };
    case "guess": {
      const t = tiles.get(a.tile);
      const target = s.players[a.target]?.name ?? "?";
      const base = `${who} annonce ${a.num} sur la ${t ? colorName(t.color) : "tuile"} de ${target}`;
      if (a.correct) return { text: `${base} : juste.`, tone: "ok" };
      const pen = a.penalty !== undefined ? tiles.get(a.penalty) : undefined;
      return { text: `${base} : raté${pen ? `, sa ${tileText(pen)} est retournée` : ""}.`, tone: "miss" };
    }
    case "stop":
      return { text: `${who} s'arrête.` };
    case "reveal":
      return { text: `${who} retourne sa ${tileText(tiles.get(a.tile))}.`, tone: "miss" };
    case "timeout":
      return { text: `Temps écoulé pour ${who}.` };
    case "out":
      return { text: `${who} est éliminé.`, tone: "out" };
    case "resign":
      return { text: `${who} abandonne.`, tone: "out" };
  }
}

function Journal({ s }: { s: GameView }) {
  const tiles = useMemo(() => {
    const m = new Map<number, TileView>();
    s.players.forEach((p) => p.row.forEach((t) => m.set(t.id, t)));
    return m;
  }, [s.players]);
  const list = useRef<HTMLOListElement>(null);
  useEffect(() => {
    const el = list.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [s.actions.length]);
  return (
    <section className="journal" aria-label="Journal de la partie">
      <h2>Journal</h2>
      {s.actions.length === 0 ? (
        <p className="muted small">Les coups s'afficheront ici.</p>
      ) : (
        <ol ref={list}>
          {s.actions.map((a, i) => {
            const l = line(s, a, tiles);
            return (
              <li key={i} className={l.tone ? `tone-${l.tone}` : ""}>
                {l.text}
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
