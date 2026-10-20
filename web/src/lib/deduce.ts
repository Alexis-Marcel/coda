import type { Action, Color, GameView, TileView } from "../api/types";

// Aide à la déduction : pour une tuile dont on ne connaît pas la valeur, les
// valeurs encore possibles d'après ce que l'on voit. Sont exclues les valeurs
// de même couleur déjà vues (ses tuiles, les tuiles retournées, sa tuile
// piochée), les annonces fausses sur cette tuile, et celles que l'ordre de la
// rangée interdit (les voisines connues l'encadrent).

export const NUMS = Array.from({ length: 12 }, (_, i) => i);

/** Valeurs vues par couleur. */
export function seen(g: GameView): [Set<number>, Set<number>] {
  const s: [Set<number>, Set<number>] = [new Set(), new Set()];
  const add = (t: TileView) => t.num !== undefined && s[t.color].add(t.num);
  g.players.forEach((p) => p.row.forEach(add));
  if (g.drawn) add(g.drawn);
  return s;
}

function misses(actions: Action[], tile: number): Set<number> {
  const out = new Set<number>();
  for (const a of actions) if (a.kind === "guess" && a.tile === tile && !a.correct) out.add(a.num);
  return out;
}

/** Bornes imposées par les voisines connues dans la rangée. */
function bounds(row: TileView[], i: number): [number, number] {
  const t = row[i];
  let lo = 0;
  let hi = 11;
  for (let j = i - 1; j >= 0; j--) {
    const l = row[j];
    if (l.num === undefined) continue;
    lo = l.color === 0 && t.color === 1 ? l.num : l.num + 1;
    break;
  }
  for (let j = i + 1; j < row.length; j++) {
    const r = row[j];
    if (r.num === undefined) continue;
    hi = t.color === 0 && r.color === 1 ? r.num : r.num - 1;
    break;
  }
  return [lo, hi];
}

export function candidates(g: GameView, seat: number, i: number, known = seen(g)): number[] {
  const row = g.players[seat].row;
  const t = row[i];
  const [lo, hi] = bounds(row, i);
  const no = misses(g.actions, t.id);
  return NUMS.filter((n) => n >= lo && n <= hi && !known[t.color].has(n) && !no.has(n));
}

/** Résumé court des possibles : « 3–7 », « 4 ou 9 », « 5 ». */
export function summary(c: number[]): string {
  if (c.length === 0) return "?";
  if (c.length === 1) return String(c[0]);
  if (c.length === 2) return `${c[0]} ou ${c[1]}`;
  return `${c[0]}–${c[c.length - 1]}`;
}

export const colorName = (c: Color, feminine = true) => (c === 0 ? (feminine ? "noire" : "noir") : feminine ? "blanche" : "blanc");
