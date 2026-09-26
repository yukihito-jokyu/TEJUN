# DatePicker

- Registry: `@yukihi/date-picker`
- Source: `registry/new-york/ui/date-picker/date-picker.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/date-picker
```

## 使用例

```tsx
import { DatePicker } from "@/components/ui/date-picker";

<DatePicker label="予定日" value={date} onValueChange={setDate} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
