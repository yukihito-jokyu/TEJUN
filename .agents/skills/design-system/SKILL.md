---
name: design-system
description: React UIをYukihito Design SystemのRegistry、既存Pattern、Foundation、Character、Tokenで実装・変更するときに使用する。新しいUIの作成、既存画面への部品導入、コンポーネント選定、配色・寸法の判断が対象。
---

# Design System

## 作業手順

1. 利用アプリの`components.json`と`@yukihi`設定を確認する。
2. [references/pattern-selection.md](references/pattern-selection.md)で主Patternと操作Patternを決め、[references/component-selection.md](references/component-selection.md)で内側の部品を選ぶ。
3. 使用する項目ごとに`references/examples/<分類>/<item>.md`を読む。ThemeProviderは`references/examples/theme-provider.md`を読む。対象が未決定なら、ファイル名または本文を検索する。
4. `npx shadcn@latest view @yukihi/<item>`で配布内容と依存を確認する。
5. `npx shadcn@latest add @yukihi/<item>`で必要な項目だけ導入する。
6. 導入されたReactファイルをこのプロジェクトのPascalCase規則へ揃え、import pathを更新する。
7. 配色・寸法・SVGを扱う場合は[references/tokens.md](references/tokens.md)を読む。
8. 実装後は[references/validation.md](references/validation.md)に従って検証する。

## 守ること

- 既存Patternで表現できる画面を低水準部品から作り直さない。
- `frontend/src/components`を雛形として先に作らない。実装する画面で必要な項目を選び、`@yukihi` Registryから導入するときだけ生成する。
- Registryから導入するcomponentを手書きで代替せず、生成後の配置と公開APIを維持する。
- React ComponentとReact Hookのファイル名はPascalCaseにする。Registryの例がkebab-caseでも、export名と振る舞いを変えずにファイル名とimport pathだけを揃える。
- Tokenにある共通値をハードコードしない。
- `registry/new-york/styles/registry.json`の承認済み値を参照し、値の変更を推測で行わない。
- SVGの形、固有色、既存の名前付きexportを変更しない。
- Portal配色、disabled、invalid、loading、IME、フォーカス復帰、Reduced Motionを維持する。
- `@yukihi/design-system`を無条件に導入せず、必要な項目だけを選ぶ。

## 例の読み方

例はコピー元ではなく、承認済みAPIの最小構成を示す。要件に合わせて文言とデータを変更してよいが、import、コンポーネント構成、アクセシビリティ属性、Token利用方法を維持する。
