"use client";
import { createContext, useContext, type ReactNode } from "react";
export type ThemeName = "milk" | "cream" | "neutral" | "sage" | "lavender";
const ThemeContext = createContext<ThemeName>("milk");
export const useDesignTheme = () => useContext(ThemeContext);

export function DesignThemeProvider({
  theme,
  children,
}: {
  theme: ThemeName;
  children: ReactNode;
}) {
  return (
    <ThemeContext.Provider value={theme}>
      <div data-ds-base={theme}>{children}</div>
    </ThemeContext.Provider>
  );
}

// shadcnのViteテンプレートはthemeプロパティなしでThemeProviderを読み込む。
// init時にテンプレートのファイルを置き換えても使えるよう、この入口を保つ。
export function ThemeProvider({ children }: { children: ReactNode }) {
  return <DesignThemeProvider theme="milk">{children}</DesignThemeProvider>;
}
