# detail-page の使用例

選択条件と状態は[仕様](../../patterns/detail-page.md)を参照する。

```tsx
import { DetailPage } from "@/components/patterns/detail-page";
<DetailPage
  title={customer.name}
  back={<a href="/customers">顧客一覧へ</a>}
  status={status}
  onRetry={reload}
>
  <CustomerDetails customer={customer} />
</DetailPage>;
```

`DetailPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
