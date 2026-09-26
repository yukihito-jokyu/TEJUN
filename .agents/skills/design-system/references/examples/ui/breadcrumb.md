# Breadcrumb

- Registry: `@yukihi/breadcrumb`
- Source: `registry/new-york/ui/breadcrumb/breadcrumb.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/breadcrumb
```

## 使用例

```tsx
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
  BreadcrumbEllipsis,
} from "@/components/ui/breadcrumb";

<Breadcrumb />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
