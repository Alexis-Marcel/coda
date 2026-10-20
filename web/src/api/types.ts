// Types du protocole, en miroir de internal/game et internal/server.

/** 0 noire, 1 blanche. */
export type Color = 0 | 1;

export interface TileView {
  id: number;
  color: Color;
  /** Absent : valeur cachée pour qui regarde. */
  num?: number;
  revealed: boolean;
}

export interface PlayerView {
  name: string;
  account: boolean;
  online: boolean;
  out: boolean;
  rank: number;
  row: TileView[];
}

export type Phase = "waiting" | "playing" | "finished" | "canceled";
export type Step = "draw" | "guess" | "choose" | "reveal";
export type Reason = "last" | "resign";

export interface Action {
  seat: number;
  kind: "draw" | "guess" | "stop" | "reveal" | "timeout" | "out" | "resign";
  color: Color;
  target: number;
  tile: number;
  num: number;
  correct?: boolean;
  penalty?: number;
  at: string;
}

export interface GameView {
  id: string;
  phase: Phase;
  you: number;
  seats: number;
  turn: number;
  step: Step | "";
  turnMs: number;
  turnLeftMs: number;
  drawn?: TileView;
  pile: [number, number];
  players: PlayerView[];
  actions: Action[];
  result?: { winner: number; reason: Reason };
  rematch: boolean[];
  next?: string;
  public: boolean;
}

export interface OpenGame {
  id: string;
  host: string;
  players: number;
  seats: number;
  turnMs: number;
  created: string;
}

export interface HistoryItem {
  id: string;
  opponents: string[];
  rank: number;
  players: number;
  reason: Reason;
  at: string;
}

export interface Me {
  name: string;
  stats: { played: number; won: number };
  games: HistoryItem[];
}

export interface CurrentGame {
  id: string;
  phase: "waiting" | "playing";
  seat: number;
  players: number;
  seats: number;
  /** Vide : en attente des autres joueurs. */
  opponents: string[];
  yourMove: boolean;
  /** Qui joue, en cours de partie. */
  turn?: string;
  out: boolean;
  created: string;
}
