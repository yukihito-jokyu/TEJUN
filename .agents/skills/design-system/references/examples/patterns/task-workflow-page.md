# task-workflow-page の使用例

選択条件と状態は[仕様](../../patterns/task-workflow-page.md)を参照する。

```tsx
import { TaskWorkflowPage } from "@/components/patterns/task-workflow-page";
<TaskWorkflowPage
  title="開設準備"
  tasks={tasks}
  completionAction={<a href="/result">結果を見る</a>}
/>;
```

`TaskWorkflowPage`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
