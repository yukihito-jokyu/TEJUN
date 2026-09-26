# Alert

- Registry: `@yukihi/alert`
- Source: `registry/new-york/ui/alert/alert.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/alert
```

## 使用例

```tsx
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";

<Alert>
  <AlertTitle>保存しました</AlertTitle>
  <AlertDescription>変更内容を反映しました。</AlertDescription>
</Alert>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
