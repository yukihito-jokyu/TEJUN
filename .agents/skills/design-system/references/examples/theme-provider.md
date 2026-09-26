# DesignThemeProvider

- Registry: `@yukihi/theme-provider`
- Source: `registry/new-york/components/theme-provider/theme-provider.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/theme-provider
```

アプリのTailwind CSSから共通スタイルを1回読み込みます。

```css
@import "tailwindcss";
/* 共通スタイルはRegistry導入時にshadcn CLIが反映します。 */
```

## 使用例

```tsx
import { DesignThemeProvider } from "@/components/theme-provider";

<DesignThemeProvider theme="sage">
  <App />
</DesignThemeProvider>;
```

テーマは`milk`、`cream`、`neutral`、`sage`、`lavender`から選ぶ。Portalを使う部品でもProviderの配色継承を維持する。
