# FileUpload

- Registry: `@yukihi/file-upload`
- Source: `registry/new-york/ui/file-upload/file-upload.tsx`

## 導入

```sh
npx shadcn@latest add @yukihi/file-upload
```

## 使用例

```tsx
import { FileUpload } from "@/components/ui/file-upload";

<FileUpload label="資料" accept=".pdf" onUpload={async (files) => upload(files)} />;
```

必要な状態・Propsは導入後の型定義で確認し、Tokenとアクセシビリティ属性を維持する。
