# AppShell

- Registry: `@yukihi/app-shell`
- Source: `registry/new-york/patterns/app-shell/app-shell.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/app-shell
```

## 使用例

```tsx
import { AppShell } from "@/components/patterns/app-shell";

<AppShell navigation={<a href="/">ホーム</a>}>
  <PageHeader title="ホーム" />
</AppShell>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
