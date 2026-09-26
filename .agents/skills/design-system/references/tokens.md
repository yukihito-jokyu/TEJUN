# Tokenと視覚仕様

- `registry/new-york/styles/registry.json`の`design-system-styles`項目を共通値とCSSの正本とする。
- 旧Token JSONの`decided`・`proposed`区分は移行済みである。未承認値を承認済みとして扱わず、値の変更は別途レビューする。
- 配色は`milk`、`cream`、`neutral`、`sage`、`lavender`から選ぶ。
- 面・文字・枠・アクセントを同じ配色セットで連動させる。
- 操作高36px、44px、48pxを維持する。
- 薄い枠4px、濃い枠2pxを維持する。
- 角丸8px、12px、20px、28pxを維持する。
- 個別SVGの形と固有色をテーマ色へ置換しない。
- StatusBubbleの独自配色を維持する。
- ASSEMBLY保存Bubbleの文言色だけ`#28180f`を維持する。
- 利用アプリの`body`や見出しへグローバルな見た目を追加しない。
- `stack`、`field`、`actions`などPatternが使う共通クラスを削除しない。
