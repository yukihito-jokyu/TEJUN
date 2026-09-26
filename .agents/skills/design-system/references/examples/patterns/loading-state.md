# LoadingState

- Registry: `@yukihi/loading-state`
- Source: `registry/new-york/patterns/loading-state/loading-state.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/loading-state
```

## 使用例

```tsx
import { LoadingState } from "@/components/patterns/loading-state";

<LoadingState loading={loading}>読み込み後の内容</LoadingState>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
