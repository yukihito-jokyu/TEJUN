# edit-page の使用例

選択条件と状態は[仕様](../../patterns/edit-page.md)を参照する。

```tsx
import { EditPage } from "@/components/patterns/edit-page";
<EditPage
  title="顧客を編集"
  variant="edit"
  onSubmit={save}
  onCancel={cancel}
  submitting={saving}
  dirty={dirty}
>
  <CustomerFields />
</EditPage>;
```

`EditPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
