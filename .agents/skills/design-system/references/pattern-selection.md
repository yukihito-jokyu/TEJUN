# 画面Pattern選択規則

画面の主目的を一つ選ぶ。複数ある場合は主操作を決め、他は補助領域にする。決められなければ画面分割を提案する。

| 主目的                               | 主Pattern                                            | 使わない条件         |
| ------------------------------------ | ---------------------------------------------------- | -------------------- |
| 指標と進行状況を把握し優先作業へ進む | [dashboard-page](patterns/dashboard-page.md)         | 数値一つの表示       |
| 複数件を探す、比較する、選ぶ         | [collection-page](patterns/collection-page.md)       | 単一対象の閲覧       |
| 1件を閲覧して関連操作へ進む          | [detail-page](patterns/detail-page.md)               | 入力と保存が主目的   |
| 1件を作成・編集して保存する          | [edit-page](patterns/edit-page.md)                   | 複数の独立した設定群 |
| 複数の設定群を管理する               | [settings-page](patterns/settings-page.md)           | 単一項目の変更       |
| 直線的な段階入力と確認               | [step-form](patterns/step-form.md)                   | 独立した作業群       |
| 独立した複数作業を進める             | [task-workflow-page](patterns/task-workflow-page.md) | 一続きのフォーム     |
| 完了結果を画面に残す                 | [result-page](patterns/result-page.md)               | 短い通知だけで十分   |

共通の画面枠は`app-shell`、AI支援が必要な場合だけ`assistant-panel`を外側へ加える。初回0件、検索0件、読み込み、エラーは主Patternの状態であり、独立した主Patternにしない。短い結果通知には`ui/toast`を使う。条件に合わない場合はPatternを作り足さず、既存部品で合成して未対応の理由を示す。

## 横断する操作

| 要件                     | 操作Pattern                                                |
| ------------------------ | ---------------------------------------------------------- |
| 条件で絞る               | [filtering](interactions/filtering.md)                     |
| 複数件に操作する         | [bulk-action](interactions/bulk-action.md)                 |
| 取り消せない操作         | [destructive-action](interactions/destructive-action.md)   |
| 入力エラーから復帰       | [validation-recovery](interactions/validation-recovery.md) |
| 対象が0件                | [empty-state](interactions/empty-state.md)                 |
| 非同期の待機・成功・失敗 | [async-feedback](interactions/async-feedback.md)           |
| 階層移動・戻り先         | [navigation](interactions/navigation.md)                   |

## 選択例

未処理件数と優先作業→`dashboard-page`。顧客を検索して選ぶ→`collection-page`＋`filtering`。顧客情報を見る→`detail-page`。顧客を作成・更新する→`edit-page`。通知・表示設定群→`settings-page`。申請を順に入力し確認する→`step-form`。数日にわたる複数の申請作業→`task-workflow-page`。登録完了後の受付番号と次の操作→`result-page`。削除確認→`destructive-action`。
