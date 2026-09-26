# collection-page

## 選択

複数件を検索・比較・選択するとき。例：顧客を検索して選ぶ／請求書を条件で絞って比較する。

使わない条件：1件の詳細を読む場合は`detail-page`。表の操作だけなら`data-table`を直接使う。

## 領域と状態

`h1`、主操作、一覧領域。標準は`DataTable`で、別形式は`renderList`に渡す。`rows`が0件なら初回空状態、検索・フィルター後の0件は表内で区別する。

初期→`loading`→一覧・初回0件・`error`。条件適用後の0件は検索・フィルター0件。条件を変えたら`filterKey`を変えてページと選択を解除し、再試行は`onRetry`で読み込みに戻す。

## データと操作

`rows`・`columns`・`getRowId`・`caption`を渡す。`filters`・`filterRow`・`filterKey`で外部条件、`onSelectionChange`で選択、`emptyAction`で次の行動を渡す。通信、認可、ルーティングは利用側が担う。原本は`registry/new-york/patterns/collection-page/collection-page.tsx`。

## 狭い画面とアクセシビリティ

狭い画面では見出し・主操作・条件・一覧の順にし、表は横スクロールを許す。検索と全選択はキーボードで操作可能にし、件数と空状態を読み上げる。
