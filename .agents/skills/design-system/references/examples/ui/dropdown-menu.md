# DropdownMenu

- Registry: `@yukihi/dropdown-menu`
- Source: `registry/new-york/ui/dropdown-menu/dropdown-menu.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/dropdown-menu
```

## 使用例

```tsx
import {
  DropdownMenu,
  DropdownMenuPortal,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from "@/components/ui/dropdown-menu";

<DropdownMenu />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
