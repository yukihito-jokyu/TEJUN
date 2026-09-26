# Accordion

- Registry: `@yukihi/accordion`
- Source: `registry/new-york/ui/accordion/accordion.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/accordion
```

## 使用例

```tsx
import {
  Accordion,
  AccordionItem,
  AccordionTrigger,
  AccordionContent,
} from "@/components/ui/accordion";

<Accordion type="single" collapsible>
  <AccordionItem value="help">
    <AccordionTrigger>ヘルプ</AccordionTrigger>
    <AccordionContent>説明です。</AccordionContent>
  </AccordionItem>
</Accordion>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
