# AlertDialog

- Registry: `@yukihi/alert-dialog`
- Source: `registry/new-york/ui/alert-dialog/alert-dialog.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/alert-dialog
```

## 使用例

```tsx
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

<AlertDialog>
  <AlertDialogTrigger asChild>
    <Button>削除する</Button>
  </AlertDialogTrigger>
  <AlertDialogContent>
    <AlertDialogHeader>
      <AlertDialogTitle>削除しますか？</AlertDialogTitle>
      <AlertDialogDescription>この操作は取り消せません。</AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>キャンセル</AlertDialogCancel>
      <AlertDialogAction variant="destructive">削除</AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
