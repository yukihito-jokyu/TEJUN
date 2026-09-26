# SoftNotice

- Registry: `@yukihi/soft-notice`
- Source: `registry/new-york/character/soft-notice/soft-notice.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/soft-notice
```

## 使用例

```tsx
import { SoftNotice } from "@/components/character/soft-notice";

<SoftNotice title="確認" tone="warning">
  公開前に内容を確認してください。
</SoftNotice>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
