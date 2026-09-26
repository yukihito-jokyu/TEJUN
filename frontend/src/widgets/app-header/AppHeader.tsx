import { FoldWelcomeCharacterIcon } from "@/components/icons/FoldWelcomeCharacterIcon";

import "./AppHeader.css";

export function AppHeader({ title, badge }: { title: string; badge?: string }) {
  return (
    <header className="app-header">
      <div className="app-header-brand">
        <span className="app-header-mark" aria-hidden="true">
          <FoldWelcomeCharacterIcon size={32} />
        </span>
        <span>{title}</span>
      </div>
      {badge && <span className="app-header-badge">{badge}</span>}
    </header>
  );
}
