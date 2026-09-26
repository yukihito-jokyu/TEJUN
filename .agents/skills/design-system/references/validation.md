# 検証

## 常に実行する

導入先アプリの既存スクリプトに合わせ、最低限、型検査、lint、production buildを実行する。

```sh
npm run typecheck
npm run lint
npm run build
```

## 操作を変更した場合

- Portal：Dialog、Select、Combobox、Toastの配色継承
- Form：disabled、invalid、loading、IME入力
- Overlay：Escape、外側操作、閉じた後のフォーカス復帰
- Calendar：日本語表記、キーボード操作、無効日
- FileUpload：accept、サイズ上限、複数選択、失敗表示
- DataTable：検索、並べ替え、選択、ページ送り、空状態
- AppShell：ナビゲーション、aside、モバイル幅
- Motion：`prefers-reduced-motion`で不要な動きを抑える

検証専用アプリ、画像、ログはこのリポジトリへ追加せず、外部一時ディレクトリで実行する。
