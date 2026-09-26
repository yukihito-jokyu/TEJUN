# AssistantPanel

- Registry: `@yukihi/assistant-panel`
- Source: `registry/new-york/patterns/assistant-panel/assistant-panel.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/assistant-panel
```

## 使用例

```tsx
import { AssistantPanel } from "@/components/patterns/assistant-panel";

<AssistantPanel title="次の一歩" body="内容を入力してください。" onClose={() => {}} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
