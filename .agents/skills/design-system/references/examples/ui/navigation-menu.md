# NavigationMenu

- Registry: `@yukihi/navigation-menu`
- Source: `registry/new-york/ui/navigation-menu/navigation-menu.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/navigation-menu
```

## 使用例

```tsx
import {
  NavigationMenu,
  NavigationMenuList,
  NavigationMenuItem,
  NavigationMenuContent,
  NavigationMenuTrigger,
  NavigationMenuLink,
  NavigationMenuIndicator,
  NavigationMenuViewport,
} from "@/components/ui/navigation-menu";

<NavigationMenu />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
