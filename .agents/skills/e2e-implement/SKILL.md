---
name: e2e-implement
description: TEJUNのmacOS向けPlaywright smoke/E2Eとfixtureを実装・実行する。
---

# E2E実装

`ponytail`を併用する。現在はmacOSだけを対象とする。受け入れ基準を利用者操作と観測可能な結果へ変換し、skipや0件成功を許さない。変更に必要な最小scenarioだけを`frontend/e2e`へ追加する。

`task e2e`はPlaywrightの`webServer`から`frontend/e2e/start-server.sh`を介して`task dev`を起動し、Wails開発サーバーへ接続する。別processの再利用を許可せず、`TEJUN_DATA_DIR`には実ユーザー領域ではなく専用一時directoryを使う。終了時にWails、Vite、Playwrightのprocessと一時directoryを片付ける。失敗時はtraceとscreenshotを`frontend/test-results`へ残す。実行コマンド、終了コード、件数、失敗、artifact、後片付けをorchestrator作業報告書へ記載する。
