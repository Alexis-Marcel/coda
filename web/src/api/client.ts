import { accessToken } from "./oidc";
import type { CurrentGame, Me, OpenGame } from "./types";

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  const token = await accessToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error ?? `erreur ${res.status}`);
  return data as T;
}

export const api = {
  config: () => request<{ oidcIssuer: string; oidcClientId: string }>("GET", "/api/config"),
  open: () => request<OpenGame[]>("GET", "/api/games/open"),
  me: () => request<Me>("GET", "/api/me"),
  current: (tokens: Record<string, string>) => request<CurrentGame[]>("POST", "/api/games/current", { tokens }),
  create: (name: string, seats: number, turnTime: number, isPublic: boolean) =>
    request<{ id: string; token: string; seat: number }>("POST", "/api/games", { name, seats, turnTime, public: isPublic }),
  join: (id: string, name: string) => request<{ token: string; seat: number }>("POST", `/api/games/${id}/join`, { name }),
};

// Jeton de place par partie, gardé dans ce navigateur.
const seatKey = (id: string) => `coda:seat:${id}`;

export function seatToken(id: string): string | null {
  try {
    return localStorage.getItem(seatKey(id));
  } catch {
    return null;
  }
}

/** Les places gardées dans ce navigateur (au plus 200). */
export function seatTokens(): Record<string, string> {
  const out: Record<string, string> = {};
  try {
    const keys = Object.keys(localStorage).filter((k) => k.startsWith("coda:seat:"));
    for (const k of keys.slice(0, 200)) out[k.slice("coda:seat:".length)] = localStorage.getItem(k) ?? "";
  } catch {
    /* stockage indisponible */
  }
  return out;
}

export function saveSeatToken(id: string, token: string) {
  try {
    localStorage.setItem(seatKey(id), token);
  } catch {
    /* navigation privée : la place tient le temps de l'onglet */
  }
}

export function guestName(): string {
  try {
    return localStorage.getItem("coda:name") ?? "";
  } catch {
    return "";
  }
}

export function saveGuestName(name: string) {
  try {
    localStorage.setItem("coda:name", name);
  } catch {
    /* rien */
  }
}
