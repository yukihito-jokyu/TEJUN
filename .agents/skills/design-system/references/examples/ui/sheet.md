# Sheet

- Registry: `@yukihi/sheet`
- Source: `registry/new-york/ui/sheet/sheet.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/sheet
```

## 使用例

```tsx
import {
  Sheet,
  SheetTrigger,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";

<Sheet />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
