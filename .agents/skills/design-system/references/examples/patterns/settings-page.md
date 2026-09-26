# settings-page の使用例

選択条件と状態は[仕様](../../patterns/settings-page.md)を参照する。

```tsx
import { SettingsPage } from "@/components/patterns/settings-page";
<SettingsPage
  title="設定"
  groups={[
    {
      id: "notice",
      title: "通知",
      content: <NotificationFields />,
      onSave: saveNotification,
      saving,
    },
  ]}
/>;
```

`SettingsPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
