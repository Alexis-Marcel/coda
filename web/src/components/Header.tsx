import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { useApp } from "../auth/AuthProvider";

export function Mark() {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className="mark">
      <rect x="3" y="5" width="12" height="22" rx="2.5" fill="#15171c" stroke="#5d6a72" />
      <rect x="17" y="5" width="12" height="22" rx="2.5" fill="#efe9dc" />
      <path d="M7 12h4M9 12v9" stroke="#efe9dc" strokeWidth="1.8" strokeLinecap="round" />
      <path d="M20.5 20.5 25.5 12" stroke="#15171c" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}

export function Header({ children }: { children?: ReactNode }) {
  const { user, loading, accounts, accountName, signIn, signOut } = useApp();
  return (
    <header className="topbar">
      <Link to="/" className="brand" aria-label="Coda, accueil">
        <Mark />
        <span>Coda</span>
      </Link>
      <div className="topbar-mid">{children}</div>
      <div className="account">
        {accounts &&
          !loading &&
          (user ? (
            <>
              <span className="account-name">{user.name}</span>
              <button className="btn btn-small btn-quiet" onClick={signOut}>
                Se déconnecter
              </button>
            </>
          ) : (
            <button className="btn btn-small" onClick={() => signIn()}>
              {accountName ? `Se connecter avec ${accountName}` : "Se connecter"}
            </button>
          ))}
      </div>
    </header>
  );
}
