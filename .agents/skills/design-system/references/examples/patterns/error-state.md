# ErrorState

- Registry: `@yukihi/error-state`
- Source: `registry/new-york/patterns/error-state/error-state.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/error-state
```

## 使用例

```tsx
import { ErrorState } from "@/components/patterns/error-state";

<ErrorState description="読み込みに失敗しました。" onRetry={() => {}} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
