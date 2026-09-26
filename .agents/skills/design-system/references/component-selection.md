# コンポーネント選択

画面の主目的と横断操作は[pattern-selection.md](pattern-selection.md)を正本として選ぶ。

## 選択順

1. [pattern-selection.md](pattern-selection.md)で画面の主Patternを決める。
2. Patternの内側を`ui`で構成する。
3. 案内や状態表現に`character`を使う。
4. 承認済みの固有SVGが必要な場合だけ`icons`を選ぶ。

| 目的               | 最初に確認する項目                                     |
| ------------------ | ------------------------------------------------------ |
| アプリ全体の枠     | `patterns/app-shell`                                   |
| 表形式の一覧領域   | `patterns/data-table`、`patterns/table-toolbar`        |
| 設定群の内側       | `patterns/settings-section`、`patterns/form-field`     |
| 検索               | `patterns/search-field`                                |
| 読み込み・エラー   | `patterns/loading-state`、`patterns/error-state`       |
| AIの案内領域       | `patterns/assistant-panel`、`character/assistant-card` |
| ファイル選択       | `ui/file-upload`                                       |
| 日付入力           | `ui/date-picker`、`ui/calendar`                        |
| 候補の選択         | 少数なら`ui/select`、検索が必要なら`ui/combobox`       |
| 確認を伴う危険操作 | `ui/alert-dialog`                                      |
| 補助的な編集領域   | `ui/sheet`                                             |
| 一時的な結果通知   | `ui/toast`                                             |

対象を決めたら`examples/<分類>/<item>.md`を読む。名称で見つからない場合は次を使う。

```sh
rg -n "目的を表す語" skills/design-system/references/examples
```
