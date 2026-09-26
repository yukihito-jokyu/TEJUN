# Popover

- Registry: `@yukihi/popover`
- Source: `registry/new-york/ui/popover/popover.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/popover
```

## 使用例

```tsx
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverAnchor,
  PopoverHeader,
  PopoverTitle,
  PopoverDescription,
} from "@/components/ui/popover";

<Popover />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
