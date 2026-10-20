import { useEffect, useState } from "react";

export function useNow(active: boolean, every = 250) {
  const [now, setNow] = useState(() => performance.now());
  useEffect(() => {
    if (!active) return;
    const t = setInterval(() => setNow(performance.now()), every);
    return () => clearInterval(t);
  }, [active, every]);
  return now;
}

/** Temps restant au tour, décompté depuis la réception de l'état. */
export function TurnTimer({ leftMs, totalMs, receivedAt }: { leftMs: number; totalMs: number; receivedAt: number }) {
  const now = useNow(totalMs > 0);
  if (totalMs <= 0) return null;
  const left = Math.max(0, leftMs - (now - receivedAt));
  const s = Math.ceil(left / 1000);
  return (
    <span className={`timer ${left < 10_000 ? "is-low" : ""}`} aria-label={`${s} secondes restantes`}>
      <span className="timer-bar" style={{ transform: `scaleX(${left / totalMs})` }} />
      <span className="timer-text">{s} s</span>
    </span>
  );
}
