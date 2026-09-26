# result-page の使用例

選択条件と状態は[仕様](../../patterns/result-page.md)を参照する。

```tsx
import { ResultPage } from "@/components/patterns/result-page";
<ResultPage
  title="受付完了"
  description="申請を受け付けました"
  status="success"
  details={<p>受付番号：{receiptId}</p>}
  action={<a href="/applications">申請一覧へ</a>}
/>;
```

`ResultPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
