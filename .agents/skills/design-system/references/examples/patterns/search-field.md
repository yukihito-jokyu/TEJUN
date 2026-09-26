# SearchField

- Registry: `@yukihi/search-field`
- Source: `registry/new-york/patterns/search-field/search-field.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/search-field
```

## 使用例

```tsx
import { SearchField } from "@/components/patterns/search-field";

<SearchField value={query} onValueChange={setQuery} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
