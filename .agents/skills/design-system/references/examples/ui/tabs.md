# Tabs

- Registry: `@yukihi/tabs`
- Source: `registry/new-york/ui/tabs/tabs.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/tabs
```

## 使用例

```tsx
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";

<Tabs defaultValue="draft">
  <TabsList>
    <TabsTrigger value="draft">下書き</TabsTrigger>
  </TabsList>
  <TabsContent value="draft">内容</TabsContent>
</Tabs>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
