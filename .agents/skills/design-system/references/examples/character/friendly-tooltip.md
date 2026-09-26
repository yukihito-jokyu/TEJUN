# FriendlyTooltip

- Registry: `@yukihi/friendly-tooltip`
- Source: `registry/new-york/character/friendly-tooltip/friendly-tooltip.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/friendly-tooltip
```

## 使用例

```tsx
import { FriendlyTooltip } from "@/components/character/friendly-tooltip";

<FriendlyTooltip content="補助説明">
  <Button>ヒント</Button>
</FriendlyTooltip>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
