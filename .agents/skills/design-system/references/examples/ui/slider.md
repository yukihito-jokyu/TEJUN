# Slider

- Registry: `@yukihi/slider`
- Source: `registry/new-york/ui/slider/slider.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/slider
```

## 使用例

```tsx
import { Slider } from "@/components/ui/slider";

<Slider defaultValue={[50]} thumbLabels={["音量"]} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
