# detail-page

## 選択

1件の内容を読み、関連操作へ進むとき。例：顧客詳細／申請内容の確認。

使わない条件：入力と保存が主目的なら`edit-page`。

## 領域と状態

戻り先、対象名の`h1`、主操作、本文。戻り先のURLや認可結果は利用側が決める。

初期→`loading`→`ready`・`missing`・`forbidden`・`error`。通信失敗だけ`onRetry`で再読み込みできる。対象なしと権限不足を混ぜない。

## データと操作

`back`に戻り先、`children`に本文、`actions`に主操作、`status`・`error`・`onRetry`に状態を渡す。通信、認可、ルーティングは利用側が担う。原本は`registry/new-york/patterns/detail-page/detail-page.tsx`。

## 狭い画面とアクセシビリティ

狭い画面でも戻り先→見出し→主操作→本文の順にする。本文の節見出しは`h2`。失敗時は説明と再試行へキーボードで到達できる。
