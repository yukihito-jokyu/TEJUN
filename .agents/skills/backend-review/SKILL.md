---
name: backend-review
description: TEJUNのGo/Wailsバックエンド差分を、正しさ、安全性、依存方向、SQLite、Binding契約についてレビューする。
---

# バックエンドレビュー

`ponytail-review`を併用する。レビューのみでは編集や自動修正をしない。`ARCHITECTURE.md`、関連仕様、対象差分、呼出し元、テストを読む。

依存逆転、transaction、入力検証、resource解放、race、不要な抽象に加えて次を確認する。

- goose v3 SQLを`embed.FS`から適用し、空DBと適用済みDBを検証している。
- SQLiteの4つのPRAGMAが接続ごとに有効である。
- 6 Serviceが同じmarshallerを使い、error causeが承認済み6項目に一致する。
- collectionがnon-nilであり、AppEventが承認済み10項目のenvelopeである。
- Binding再生成後に`frontend/bindings`の差分が残らない。

Binding契約を変更する場合は`docs/approved/technology-selection.md`と`docs/approved/binding-data-contracts.md`を正本とする。コードコメントは必要な日本語だけとし、不要な説明や英語コメントが追加されていないかも確認する。言語・ツールが要求するdirectiveや自動生成コメントは対象外とする。`task lint`、`go test -race ./...`、`task bindings:check`を実行し、指摘は重要度、発生条件、影響、絶対パス・行番号、短い引用、修正方向を示す。問題がなければ確認範囲と未検証事項を報告する。
