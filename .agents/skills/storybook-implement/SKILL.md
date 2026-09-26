---
name: storybook-implement
description: TEJUNのStorybook設定とReact storyを追加・変更し、既存UIの代表状態を独立表示できるようにする。
---

# Storybook実装

`frontend-implement`、`design-system`、`ponytail`を必ず併用する。開始前に`ARCHITECTURE.md`、`frontend/.storybook`、対象Componentと近接するtest・story、`frontend/package.json`、既存差分を読む。

`.agents/skills/design-system/SKILL.md`と配下のreferenceは変更しない。

- storyは対象Componentの隣へ`<Component>.stories.tsx`として置き、Componentと同じ公開APIを利用する。
- 実在するUIの代表状態だけを追加し、Storybook専用のサンプルUI、将来用Component、空directoryを作らない。
- Design SystemのRegistry、Pattern、Foundation、Character、Tokenを優先する。該当部品がなく独自デザインする場合は、orchestrator作業報告書の「Design System例外」へ該当箇所、調査候補、不採用理由、独自実装内容を書く。
- argsで表せる状態はargsを使う。Provider、router、Wails境界の差し替えは対象storyの表示に必要な最小範囲だけdecoratorまたはmockへ置く。
- Wails生成Bindingをstoryから直接importしない。アプリコードと同じ`src/shared/api/wails`境界を守る。
- Storybookの自動生成サンプル、telemetry、不要なaddonを追加しない。依存を追加する場合はversionを固定する。
- global styleやThemeProviderは本番と同じ入口を再利用し、story専用に見た目を再実装しない。
- play functionは利用者操作の回帰確認が必要な場合だけ追加し、unit testやE2Eと同じ検証を重複させない。
- コメントは必要な場合だけ日本語で書く。設定仕様上必要な記述と自動生成コメントは対象外とする。

完了前に`npm --prefix frontend run typecheck`、`lint`、`fmt:check`、`test`、`build`、`storybook:build`を実行する。Storybook設定や起動方法を変更した場合は`npm --prefix frontend run storybook -- --smoke-test --ci`も実行し、Design System例外の有無を報告する。
