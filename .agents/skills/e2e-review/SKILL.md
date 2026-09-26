---
name: e2e-review
description: TEJUNのmacOS向けPlaywright E2Eを、要件対応、偽陽性、分離、cleanup、実行証跡についてレビューする。
---

# E2Eレビュー

`ponytail-review`を併用する。レビューのみでは編集しない。macOSで`task e2e`がVite開発サーバーを所有することを確認する。要件を観測しているか、skip・0件・mock正常応答・別processの再利用だけで成功していないか、再実行可能かを確認する。終了後にportとprocessが残らないことと、失敗時artifactが妥当なことも確認する。結果は根拠付きでorchestrator作業報告書へ記載する。
