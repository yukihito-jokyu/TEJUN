---
name: orchestrator
description: TEJUNのIssue実装を調査、承認、実装、レビュー、audit、E2Eへ分割し、サブエージェントの作業報告書と進捗を管理する。
---

# Issue実装のオーケストレーション

メインはIssue受付、担当分割、承認、報告統合を行い、調査・実装・レビュー・検証は新規サブエージェントへ委譲する。サブエージェントを使えない場合は代行完了にせず制約を報告する。

## 必須フロー

Issueを受け付けたら、実装を始める前に次の順序を守る。

1. Issue本文、受け入れ基準、関連Issueを確認する。
2. 調査対象を分割し、新規サブエージェントへ並行委譲する。最低限、対象コードと呼出し元、既存テスト・Storybook・E2E、仕様書とアーキテクチャ、変更境界とリスクを調べる。各担当は事実、推測、未確定事項、根拠を分けてログへ残す。
3. 調査結果を統合し、[計画テンプレート](assets/plan-template.md)から`docs/work/<Issue番号>/plan.md`を作る。実装担当を起動する前に、計画の矛盾、共有ファイル競合、要件とテストの対応漏れを確認する。
4. `plan.md`の内容を人間へ提示し、明示承認を求める。承認前に許可される変更は`docs/work/<Issue番号>`内の計画と調査ログだけであり、製品コード、テスト、Story、管理対象ドキュメントを変更しない。
5. 承認後、計画版を確定し、先行条件を満たす独立作業を可能な限り並行して新規サブエージェントへ委譲する。共有ファイルまたは同じ実行環境を使う作業だけを直列化する。
6. 実装、独立レビュー、修正、再レビュー、audit/debt、E2E、cleanupを計画どおり進め、結果に応じて`plan.md`の状態とログを更新する。

調査でIssue情報を取得できない、または計画を成立させる重要事項が未確定の場合は、推測で埋めず、人間判断待ちとして`plan.md`に記録して返す。承認後に要件、担当境界、テスト方針が実質的に変わった場合は計画版を上げ、影響範囲の実装を止めて再承認を得る。

## 成果物

`docs/work/<Issue番号>/plan.md`と`logs/<連番>-<作業>.md`を使う。`docs/work`はGit管理対象外のままとし、`.gitignore`を変更しない。既存成果物を消さず、作業ID、先行条件、担当スキル、状態、予定ログ、通常レビューと最終チェックの差し戻し回数を管理する。[計画テンプレート](assets/plan-template.md)を使って、受け入れ基準、実装作業、各テスト、Storybook Story、E2Eシナリオを数字IDで対応付ける。計画を提示し、人間の明示承認後だけ実装へ進む。

計画には変更予定の層・ファイル・契約、各サブエージェントの担当と使用スキル、起動順、先行条件、並行可能な組合せ、共有資源の所有、レビュー担当、検証コマンド、cleanupを具体化する。テストは単体、コンポーネント、契約・Binding、統合、回帰、E2Eについて、対象、初期状態、入力・操作、期待結果、異常・境界条件、fixture/mock、実行方法、証跡を記載する。該当しない種類は黙って省略せず、不要な理由を書く。

Frontend変更ではStorybookの既存有無と近いStoryを調査する。Storyを追加・更新する場合は、対象component、各Story名、props/fixture、表示状態、interaction、viewport、a11y・visual確認、要件との対応を計画へ書く。Storybookがない、またはStoryが適さない場合は、その根拠と代替テストを書く。Storybook基盤の新規導入が必要なら暗黙に追加せず、独立した作業・影響・検証として計画し、人間の承認対象に含める。

各担当は[作業報告テンプレート](assets/log-template.md)を使う。Frontend担当は「Design System例外」を必ず記載する。`.agents/skills/design-system`に要件を満たす部品がなく独自デザインした場合は、該当箇所、調査した候補、不採用理由、独自実装内容を書く。例外がなければ「なし」とする。Frontendレビューはこの記録と実装を照合する。

## 担当

| 作業 | 実装 | レビュー |
|---|---|---|
| Go/Wails | `backend-implement` + `ponytail` | `backend-review` + `ponytail-review` |
| React | `frontend-implement` + `design-system` + `ponytail` | `frontend-review` + `design-system` + `ponytail-review` |
| Storybook | `storybook-implement` + `frontend-implement` + `design-system` + `ponytail` | `storybook-review` + `frontend-review` + `design-system` + `ponytail-review` |
| macOS E2E | `e2e-implement` + `ponytail` | `e2e-review` + `ponytail-review` |

共有ファイルを変更する担当は直列化する。調査、実装、修正、レビュー、再レビュー、audit、debt、E2Eは作業ごとに新規サブエージェントへ割り当て、別作業へ使い回さない。レビュー担当は修正せず、差し戻しは新規実装担当へ渡す。通常レビューと最終auditの差し戻しは元作業ごとに数え、各2回で自動修正を止めて人間へ返す。

全実装レビュー後に`ponytail-audit`と`ponytail-debt`を別担当で実行し、必要な修正・再レビュー後にmacOS E2Eを実行・レビューする。全受け入れ基準、レビュー、audit/debt、E2E、cleanupをログと照合して完了判定する。

## 形式検査

構造化JSONで計画や遷移を検査する場合だけ、[入力仕様](references/check-input.md)を読み、`scripts/check_plan.py`を使う。通常はMarkdownのplanとlogsだけを正とし、JSONの二重管理を必須にしない。形式検査は実際の承認、agent起動、シナリオ網羅、実装品質を保証しない。
