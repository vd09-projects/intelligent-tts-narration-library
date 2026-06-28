// AppHeader.tsx — the single <header> landmark (plan Semantics & Landmarks).
// h1 = "Earshot". Carries the mobile session-pane collapse toggle (CSS-only
// collapse; aria-expanded reflects state).

export function AppHeader({
  sessionOpen,
  onToggleSession,
}: {
  sessionOpen: boolean;
  onToggleSession: () => void;
}) {
  return (
    <header className="app-header">
      <button
        type="button"
        className="app-header__session-toggle"
        aria-expanded={sessionOpen}
        aria-controls="session-pane"
        onClick={onToggleSession}
      >
        <span aria-hidden="true">☰</span>
        <span className="visually-hidden">
          {sessionOpen ? "Hide sessions" : "Show sessions"}
        </span>
      </button>
      <h1 className="app-header__title">Earshot</h1>
      <p className="app-header__tagline">Listen to it, don&rsquo;t read it.</p>
    </header>
  );
}
