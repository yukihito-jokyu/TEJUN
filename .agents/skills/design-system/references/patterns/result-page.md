# result-page

## 選択

完了結果と次の行動を画面に残すとき。例：登録番号と対象リンク／一部成功した一括処理の結果。

使わない条件：短い通知だけで十分なら`ui/toast`。

## 領域と状態

`h1`、結果の説明、詳細、対象へのリンク、次の主操作。

`success`は完了、`partial`は一部完了、`failure`は失敗。`partial`と`failure`で`onRetry`を渡せば再試行できる。通知の寿命に依存せず結果を残す。

## データと操作

`status`・`description`・`details`に結果、`targetLink`・`action`に次の行動、`onRetry`に再試行を渡す。通信、認可、ルーティングは利用側が担う。原本は`registry/new-york/patterns/result-page/result-page.tsx`。

## 狭い画面とアクセシビリティ

狭い画面では説明→詳細→操作の順にする。結果は`role="status"`で読み上げ、再試行と次の行動へキーボードで到達できる。
