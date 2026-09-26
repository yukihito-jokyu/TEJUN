# Pagination

- Registry: `@yukihi/pagination`
- Source: `registry/new-york/ui/pagination/pagination.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/pagination
```

## 使用例

```tsx
import {
  Pagination,
  PaginationContent,
  PaginationLink,
  PaginationItem,
  PaginationPrevious,
  PaginationNext,
  PaginationEllipsis,
} from "@/components/ui/pagination";

<Pagination />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
