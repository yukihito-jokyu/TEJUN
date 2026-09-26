# collection-page の使用例

選択条件と状態は[仕様](../../patterns/collection-page.md)を参照する。

```tsx
import { CollectionPage } from "@/components/patterns/collection-page";
<CollectionPage
  title="顧客"
  rows={customers}
  columns={columns}
  getRowId={(row) => row.id}
  caption="顧客一覧"
  filterKey={conditionKey}
/>;
```

`CollectionPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
