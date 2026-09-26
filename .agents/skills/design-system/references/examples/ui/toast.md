# toast

- Registry: `@yukihi/toast`
- Source: `registry/new-york/ui/toast/toast.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/toast
```

## 使用例

```tsx
import { toast } from "@/components/ui/toast";

<Button onClick={() => toast.success("保存しました")}>保存する</Button>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
