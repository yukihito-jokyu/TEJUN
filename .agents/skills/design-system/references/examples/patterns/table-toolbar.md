# TableToolbar

- Registry: `@yukihi/table-toolbar`
- Source: `registry/new-york/patterns/table-toolbar/table-toolbar.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/table-toolbar
```

## 使用例

```tsx
import { TableToolbar } from "@/components/patterns/table-toolbar";

<TableToolbar query={query} onQueryChange={setQuery} count={10} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
