# dashboard-page の使用例

選択条件と状態は[仕様](../../patterns/dashboard-page.md)を参照する。

```tsx
import { DashboardPage } from "@/components/patterns/dashboard-page";
<DashboardPage
  title="業務概要"
  metrics={[{ label: "未処理", value: pendingCount }]}
  actions={<button onClick={openQueue}>作業へ</button>}
/>;
```

`DashboardPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
