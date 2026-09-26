# DataTable

- Registry: `@yukihi/data-table`
- Source: `registry/new-york/patterns/data-table/data-table.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/data-table
```

## 使用例

```tsx
import { DataTable } from "@/components/patterns/data-table";

<DataTable
  rows={[{ id: "1", name: "文書" }]}
  columns={[{ id: "name", header: "名前", value: (row) => row.name }]}
  getRowId={(row) => row.id}
  caption="文書一覧"
/>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
