# Tooltip

- Registry: `@yukihi/tooltip`
- Source: `registry/new-york/ui/tooltip/tooltip.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/tooltip
```

## 使用例

```tsx
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";

<Tooltip>
  <TooltipTrigger asChild>
    <Button>ヒント</Button>
  </TooltipTrigger>
  <TooltipContent>補助説明</TooltipContent>
</Tooltip>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
