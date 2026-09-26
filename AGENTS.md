# TEJUNで作業するとき

このファイルは作業開始時の入口である。ファイル配置と依存方向の正本は[ARCHITECTURE.md](ARCHITECTURE.md)とし、ここへ同じ規則を複製しない。

## 作業前に確認すること

1. 対象Issueの要件と受け入れ基準を確認する。
2. `git status --short`で既存差分を確認し、他の作業を上書きしない。
3. [README.md](README.md)、[ARCHITECTURE.md](ARCHITECTURE.md)、対象コード、呼出し元、近いテストを読む。
4. 対象に合う`.agents/skills`を読み、その実装・レビュー・検証手順に従う。
5. 配置を増やす場合は、ARCHITECTURE.mdの「ファイルを増やす判断」に従う。

人間へ根拠を示すときは、実際の記述を短く引用し、絶対pathと行番号へのlinkを添える。説明は前提、判断、結果の順につなげる。

## 作業別の入口

| 作業 | 使用するskill | 最初に確認する場所 |
| --- | --- | --- |
| Issue全体の実装 | `.agents/skills/orchestrator` | `docs/work/<Issue番号>/plan.md`、対象Issue |
| Go・Wails・SQLite実装 | `.agents/skills/backend-implement` | `internal/domain`、`internal/application`、`internal/adapter`、`internal/bootstrap` |
| Go・Wails・SQLiteレビュー | `.agents/skills/backend-review` | 実装差分、呼出し元、同packageのテスト |
| React実装 | `.agents/skills/frontend-implement`と`.agents/skills/design-system` | `frontend/src`、`components.json`、`oxlint.config.ts` |
| Reactレビュー | `.agents/skills/frontend-review`と`.agents/skills/design-system` | 実装差分、Design System候補、Frontendテスト |
| Storybook実装 | `.agents/skills/storybook-implement` | `frontend/.storybook`、対象Component、近接するtest・story |
| Storybookレビュー | `.agents/skills/storybook-review` | Storybook差分、対象Component、作業報告書 |
| macOS E2E実装 | `.agents/skills/e2e-implement` | `frontend/e2e`、`playwright.config.ts`、`build/config.yml` |
| macOS E2Eレビュー | `.agents/skills/e2e-review` | E2E差分、artifact、cleanup |

実装とレビューは同じ担当で完結させない。FrontendではDesign Systemの推奨componentを優先し、該当componentがなく独自デザインした場合はorchestrator作業報告書の「Design System例外」へ記録する。

## 実装時の共通ルール

- 未確定のBinding、API、directoryを推測で追加しない。
- 空package、将来用interface、単一実装のためだけのwrapperを作らない。
- 生成された`frontend/bindings`と`cmd/tejun/frontend/dist`を手編集しない。
- コメントは必要な場合だけ日本語で書く。directive、型参照、自動生成コメントは変更しない。
- Goテストはテーブル駆動とし、対象実装と同じpackageへ置く。
- React ComponentとReact Hookのファイル名はPascalCaseにし、Hook関数名はlower camel caseにする。`main.tsx`などのentry pointは対象外とする。
- UIはWails Bindingを`frontend/src/shared/api/wails`経由で利用する。
- `frontend/src/components`は必要なDesign System項目をRegistryから導入するときだけ生成し、独自componentや先行雛形を置かない。
- `.agents/skills/design-system/SKILL.md`と配下のreferenceは変更しない。プロジェクト固有規則はARCHITECTURE.mdとFrontend用skillへ記載する。
- `docs/work`はGit管理対象外の作業記録として扱い、docsのignore規則を変更しない。
- 規約、配置、検証commandを変更した場合はREADME、ARCHITECTURE.md、関連skillも同じ作業で確認する。

## 検証

変更範囲に応じて個別commandを実行し、完了前に原則として`task check`を実行する。

| command | 検証内容 |
| --- | --- |
| `task fmt` | GoとFrontendの整形 |
| `task lint` | GoとFrontendの静的検査、整形差分 |
| `task test` | Go race testとFrontend unit test |
| `task bindings:check` | Wails Bindingの再生成差分と追跡漏れ |
| `task build` | FrontendとmacOS用Wails binary |
| `task e2e` | Vite開発サーバーを使うPlaywright |
| `task check` | 上記のうち自動検証対象を一括実行 |

実行していない検証、warning、残作業がある場合は完了報告で明示する。
