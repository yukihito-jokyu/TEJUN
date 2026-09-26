---
name: storybook-review
description: TEJUNのStorybook設定とReact storyの差分を、再現性、依存境界、状態表現、Design System準拠についてレビューする。
---

# Storybookレビュー

`frontend-review`、`design-system`、`ponytail-review`を必ず併用する。レビューのみでは実装ファイルを編集しない。開始前に関連仕様、差分、対象Component、近接するtest・story、Storybook設定、orchestrator作業報告書を読む。

`.agents/skills/design-system/SKILL.md`と配下のreferenceは変更しない。

- storyが実在するComponentの公開APIを使い、代表状態を利用者に分かる入力で再現しているか確認する。
- Storybook専用サンプルUI、将来用Component、不要なaddon・dependency・decoratorが追加されていないか確認する。
- Provider、router、Wails境界のmockが必要最小限で、生成Bindingの直接importや本番と異なるデータ契約を持ち込んでいないか確認する。
- 本番と同じglobal style、ThemeProvider、Design System部品、Tokenを利用しているか確認する。
- loading、empty、error、disabled、invalidなどは対象Componentが実際に持つ状態だけを確認し、存在しない状態の追加を要求しない。
- React Componentとstoryのファイル名がPascalCaseで、storyが対象Componentの隣にあることを確認する。
- 自動生成サンプル、telemetry、Storybook専用の独自デザインがないことを確認する。独自デザインが必要な場合は、作業報告書の「Design System例外」と実装内容が一致することを合格条件にする。
- unit test、play function、E2Eの検証責務が不必要に重複していないか確認する。

Frontendのtypecheck、lint、format check、unit test、production build、Storybook静的buildを実行する。設定や起動処理の差分がある場合はStorybook smoke testも実行する。指摘は重要度、再現条件、影響、根拠、修正方向を示し、警告と修正必須事項を分けて報告する。
