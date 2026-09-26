# empty-state

## 適用条件

初回または検索・フィルター後に0件になる場合。通信失敗には適用しない。

## 操作と状態

`character/empty-state`の`kind`を初回`initial`、検索`search`、フィルター`filter`から選ぶ。初回は作成などの開始操作、条件適用後は条件解除を示す。`collection-page`の`emptyAction`に行動を渡せる。

## 狭い画面とアクセシビリティ

狭い画面でも説明と行動を一列にする。空状態を読み上げ、行動へキーボードで到達できる。
