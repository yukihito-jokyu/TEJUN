# IconButton

- Registry: `@yukihi/icon-button`
- Source: `registry/new-york/ui/icon-button/icon-button.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/icon-button
```

## 使用例

```tsx
import { IconButton } from "@/components/ui/icon-button";

<IconButton aria-label="閉じる">
  <XIcon size={20} />
</IconButton>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
