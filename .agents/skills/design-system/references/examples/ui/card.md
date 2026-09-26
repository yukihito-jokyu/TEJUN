# Card

- Registry: `@yukihi/card`
- Source: `registry/new-york/ui/card/card.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/card
```

## 使用例

```tsx
import {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
} from "@/components/ui/card";

<Card>
  <CardHeader>
    <CardTitle>設定</CardTitle>
  </CardHeader>
  <CardContent>内容</CardContent>
</Card>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
