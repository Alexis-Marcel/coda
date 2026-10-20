import { accessToken } from "./oidc";
import type { Me, OpenGame } from "./types";

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
