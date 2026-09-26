# Dialog

- Registry: `@yukihi/dialog`
- Source: `registry/new-york/ui/dialog/dialog.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/dialog
```

## 使用例

```tsx
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

<Dialog>
  <DialogTrigger asChild>
    <Button>詳細を開く</Button>
  </DialogTrigger>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>詳細</DialogTitle>
      <DialogDescription>内容を確認します。</DialogDescription>
    </DialogHeader>
  </DialogContent>
</Dialog>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
