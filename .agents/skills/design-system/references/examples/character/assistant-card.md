# AssistantCard

- Registry: `@yukihi/assistant-card`
- Source: `registry/new-york/character/assistant-card/assistant-card.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/assistant-card
```

## 使用例

```tsx
import { AssistantCard } from "@/components/character/assistant-card";

<AssistantCard
  title="次の一歩"
  body="内容を入力してください。"
  suggestions={[{ label: "ヒント", onSelect: () => {} }]}
/>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
