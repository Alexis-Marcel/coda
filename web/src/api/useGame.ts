import { useCallback, useEffect, useRef, useState } from "react";
import { accessToken } from "./oidc";
import type { GameView } from "./types";

export type Conn = "connecting" | "open" | "closed";

export interface GameSocket {
  state: GameView | null;
  /** Instant de réception de l'état (pour faire tourner les pendules). */
  receivedAt: number;
  conn: Conn;
  error: string | null;
  notFound: boolean;
  send: (cmd: Record<string, unknown>) => void;
  clearError: () => void;
}

// Une connexion par partie, rétablie avec un délai croissant. Chaque message
// d'état porte toute la partie : rien à rattraper après une coupure.
export function useGame(id: string, token: string | null): GameSocket {
  const [state, setState] = useState<GameView | null>(null);
  const [receivedAt, setReceivedAt] = useState(0);
  const [conn, setConn] = useState<Conn>("connecting");
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);
  const ws = useRef<WebSocket | null>(null);

  useEffect(() => {
    let stopped = false;
    let attempt = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const connect = async () => {
      setConn("connecting");
      const q = new URLSearchParams();
      if (token) q.set("token", token);
      const jwt = await accessToken();
      if (jwt) q.set("access_token", jwt);
      if (stopped) return;
      const proto = window.location.protocol === "https:" ? "wss" : "ws";
      const sock = new WebSocket(`${proto}://${window.location.host}/ws/games/${id}?${q}`);
      ws.current = sock;
      let opened = false;
      sock.onopen = () => {
        opened = true;
        attempt = 0;
        setConn("open");
      };
      sock.onmessage = (ev) => {
        const msg = JSON.parse(ev.data);
        if (msg.t === "state") {
          setState(msg.state);
          setReceivedAt(performance.now());
        } else if (msg.t === "error") {
          setError(msg.message);
        }
      };
      sock.onclose = async () => {
        if (ws.current === sock) ws.current = null;
        if (stopped) return;
        setConn("closed");
        if (!opened) {
          // Refus avant ouverture : partie inconnue ?
          const res = await fetch(`/api/games/${id}`).catch(() => null);
          if (res?.status === 404) {
            setNotFound(true);
            return;
          }
        }
        attempt++;
        timer = setTimeout(connect, Math.min(500 * 2 ** attempt, 10_000) * (0.7 + Math.random() * 0.6));
      };
    };
    void connect();
    return () => {
      stopped = true;
      clearTimeout(timer);
      ws.current?.close();
    };
  }, [id, token]);

  const send = useCallback((cmd: Record<string, unknown>) => {
    setError(null);
    if (ws.current?.readyState === WebSocket.OPEN) ws.current.send(JSON.stringify(cmd));
    else setError("Connexion perdue : nouvelle tentative en cours.");
  }, []);

  return { state, receivedAt, conn, error, notFound, send, clearError: () => setError(null) };
}
