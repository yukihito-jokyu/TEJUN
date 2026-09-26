---
name: backend-implement
description: TEJUNのGo/Wailsバックエンドを実装・修正し、承認済みの依存方向と検証手順に沿って仕上げる。
---

# バックエンド実装

`ponytail`を併用する。開始前に`ARCHITECTURE.md`、`go.mod`、`.golangci.yml`、`Taskfile.yml`、対象コード・呼出し元・テスト、既存差分を読む。

- `bootstrap`だけがadapterを生成・接続する。
- `domain`は外部技術に依存させず、`application`は必要最小限のportだけを要求する。
- Wails Serviceは入力検証、DTO変換、application呼出しだけを担う。
- SQLite migrationはgoose v3 SQLとして`internal/adapter/sqlite/migration`へ置き、`embed.FS`から空DBと適用済みDBへ適用して検証する。
- SQLiteの全接続で`foreign_keys=ON`、`journal_mode=WAL`、`synchronous=FULL`、`busy_timeout=5000`を有効にする。
- 6 Serviceへ同じ`ServiceOptions.MarshalError`を明示し、公開collectionはnon-nilにする。
- error causeは`code`、`message`、`fieldErrors`、`retryable`、`causeId`、`currentRevision`を使用する。
- AppEventは承認済み10項目のenvelopeと`correlation`を使用する。変更時は`docs/approved/technology-selection.md`と`docs/approved/binding-data-contracts.md`を読む。
- Binding変更後は再生成し、`frontend/bindings`の追跡済み差分を確認する。
- 空の将来用package、単一実装用interface、目的のないwrapperを作らない。
- コードコメントは必要な場合だけ日本語で書く。言語・ツールが要求するdirectiveや自動生成コメントは変更しない。
- 処理のまとまりと宣言の境界には読みやすい空行を置き、`wsl_v5`、`gofumpt`、`goimports`、`golines`の結果を正とする。
- Goテストはケース名、入力、期待値をまとめたテーブル駆動テストにし、各ケースを`t.Run`で実行する。

完了前に`task fmt`、`task lint`、`go test -race ./...`、`task bindings:check`、影響があれば`task build`を実行する。既存変更を上書きせず、変更・検証・未確認事項を報告する。
