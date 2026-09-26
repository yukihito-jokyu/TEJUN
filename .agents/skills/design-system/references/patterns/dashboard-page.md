# dashboard-page

## 選択

複数の指標や進行状況を俯瞰し、優先作業を選ぶとき。例：未処理件数から担当案件へ進む／監視指標から障害対応へ進む。

使わない条件：数値カード1枚だけ、または一覧から対象を選ぶことが主目的の画面。後者は`collection-page`。

## 領域と状態

`PageHeader`の`h1`、主要指標のカード群、状態別情報と次の行動を`sections`に配置する。指標の意味、更新頻度、行動先は利用側が渡す。

初期→`loading`→表示または`error`。`error`から`onRetry`で再読み込みへ戻る。部分領域の待機・失敗は`sections`内で`LoadingState`・`ErrorState`を使い、他の指標を残す。

## データと操作

`metrics`にラベル・値・説明、`actions`と`sections`に行動、`loading`・`error`・`onRetry`に非同期状態を渡す。通信、認可、ルーティングは利用側が担う。原本は`registry/new-york/patterns/dashboard-page/dashboard-page.tsx`。

## 狭い画面とアクセシビリティ

狭い画面では指標カードを1列にし、見出し→指標→行動の順に読む。カード見出しは`h2`。行動先はキーボードで到達可能にし、更新時にフォーカスを奪わない。
