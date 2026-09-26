# CharacterIcon・FeatureIcon・StatusBubble

- Registry: `@yukihi/character-icons`
- Source: `registry/new-york/character/character-icons/index.tsx`

## 導入

動的に6方向のアイコンを選択するときだけ使用します。個別SVGだけが必要なら、対応する個別itemを導入します。

```sh
npx shadcn@latest add @yukihi/character-icons
```

## 使用例

```tsx
import { CharacterIcon, FeatureIcon, StatusBubble } from "@/components/character";

<CharacterIcon direction="fold" expression="welcome" label="ようこそ" />;
<FeatureIcon direction="ink" name="idea" label="アイデア" />;
<StatusBubble direction="assembly" status="saved" announce />;
```

個別アイコンの例は`../icons/`を参照する。
