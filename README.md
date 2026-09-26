# TEJUN

AIと人間が動作確認と証跡を揃え、検証済みの手順書を作成するWailsアプリケーション。

要件、画面導線、Wails Binding、データ契約、技術選定、検証結果は[Documentation](./docs/README.md)から参照する。

実装に取りかかるときは[AGENTS.md](./AGENTS.md)を入口とし、ファイル配置と依存方向は[ARCHITECTURE.md](./ARCHITECTURE.md)に従う。

## 開発

必要なツールはGo 1.26.6、Node 24.9.0、npm 11.12.1、Task 3.45.5、golangci-lint 2.12.2である。

```sh
npm ci --prefix frontend
npx --prefix frontend playwright install chromium
task dev
```

`task dev`はこの作業ツリーの`.tejun-dev`をデータ領域としてWailsアプリとVite開発サーバーを起動する。既存データ領域を使う場合は`TEJUN_DATA_DIR`で明示的に上書きできる。`task storybook`は既存React UIを確認するStorybookを起動する。全検査は`task check`、macOS向けPlaywright smoke testは`task e2e`、productionバイナリ生成は`task build`で実行する。`task check`にはStorybookの静的buildも含む。生成物は`bin/tejun`へ配置する。配置と依存方向は[ARCHITECTURE.md](./ARCHITECTURE.md)を参照する。
