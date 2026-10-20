import type { TileView } from "../api/types";
import { colorName } from "../lib/deduce";

export type TileSize = "l" | "m" | "s";

interface Props {
  tile: Pick<TileView, "color" | "num" | "revealed">;
  size?: TileSize;
  /** Sa propre tuile encore cachée : on en voit la valeur, les autres non. */
  mine?: boolean;
  hint?: string;
  selected?: boolean;
  fresh?: boolean;
  onClick?: () => void;
  label?: string;
}

// Une tuile debout, vue de face : noire ou blanche, la valeur gravée au milieu.
// Retournée, elle porte un trait en laiton sous la valeur : tout le monde la voit.
export function Tile({ tile, size = "m", mine, hint, selected, fresh, onClick, label }: Props) {
  const known = tile.num !== undefined;
  const cls = [
    "tile",
    `tile-${size}`,
    tile.color === 0 ? "tile-black" : "tile-white",
    tile.revealed ? "is-revealed" : "",
    mine && !tile.revealed ? "is-mine" : "",
    !known ? "is-hidden" : "",
    selected ? "is-selected" : "",
    fresh ? "is-fresh" : "",
    onClick ? "is-clickable" : "",
  ].join(" ");
  const aria =
    label ??
    `${colorName(tile.color)} ${known ? tile.num : "cachée"}${tile.revealed ? ", retournée" : mine ? ", cachée aux autres" : ""}`;
  const face = (
    <>
      <span className="tile-num">{known ? tile.num : ""}</span>
      {hint !== undefined && <span className="tile-hint">{hint}</span>}
    </>
  );
  return onClick ? (
    <button type="button" className={cls} onClick={onClick} aria-label={aria} aria-pressed={selected}>
      {face}
    </button>
  ) : (
    <span className={cls} role="img" aria-label={aria}>
      {face}
    </span>
  );
}
