---
name: frontend-review
description: TEJUNのReact差分をWails境界、型安全性、操作性、Yukihito Design System準拠についてレビューする。
---

# フロントエンドレビュー

`ponytail-review`と`design-system`を必ず併用する。レビューのみでは編集しない。

関連仕様、差分、呼出し元、テストを読み、依存方向、aliasと相対pathのBinding直接import、nullable、構造化error、Event cleanup、非同期競合、アクセシビリティを確認する。FrontendとE2EのTypeScriptがともに型検査対象であることも確認する。React ComponentとReact Hookのファイル名がPascalCaseで、Hook関数名がlower camel caseであることを確認する。`main.tsx`などのentry pointはPascalCase検査の対象外とする。コードコメントは必要な日本語だけとし、不要な説明や英語コメントが追加されていないかも確認する。directive、型参照、自動生成コメントは対象外とする。`frontend/src/components`に手書きの独自componentや先行雛形がないことを確認する。Design Systemの推奨部品で実現できる独自実装は差し戻す。独自デザインが必要な場合は、orchestrator作業報告書の「Design System例外」に該当箇所、候補、不採用理由、実装内容があることを合格条件にする。

Frontendのtypecheck、lint、format check、test、buildを実行し、指摘は重要度、条件、影響、根拠、修正方向を示す。
