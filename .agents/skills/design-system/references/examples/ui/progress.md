# Progress

- Registry: `@yukihi/progress`
- Source: `registry/new-york/ui/progress/progress.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/progress
```

## 使用例

```tsx
import { Progress } from "@/components/ui/progress";

<Progress value={60} aria-label="進捗" />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
