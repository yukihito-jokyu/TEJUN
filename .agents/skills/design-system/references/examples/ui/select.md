# Select

- Registry: `@yukihi/select`
- Source: `registry/new-york/ui/select/select.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/select
```

## 使用例

```tsx
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectScrollDownButton,
  SelectScrollUpButton,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

<Select defaultValue="draft">
  <SelectTrigger aria-label="状態">
    <SelectValue />
  </SelectTrigger>
  <SelectContent>
    <SelectItem value="draft">下書き</SelectItem>
  </SelectContent>
</Select>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
