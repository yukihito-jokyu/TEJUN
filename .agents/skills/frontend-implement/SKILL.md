---
name: frontend-implement
description: TEJUNのReact UIをWails境界とYukihito Design Systemに沿って実装・修正する。
---

# フロントエンド実装

`ponytail`と`design-system`を必ず併用する。開始前に`ARCHITECTURE.md`、`frontend/package.json`、lint・format設定、対象コード・テスト、既存差分を読む。

`.agents/skills/design-system/SKILL.md`と配下のreferenceは変更しない。TEJUN固有の規則はこのskillまたは`ARCHITECTURE.md`へ記載する。

- UIは`.agents/skills/design-system/SKILL.md`のRegistry、既存Pattern、Foundation、Character、Tokenが推奨する部品を優先する。
- `frontend/src/components`へ独自componentや先行雛形を作らない。対象画面に必要な項目だけを`@yukihi` Registryから導入する。
- 該当部品が存在しない場合だけ独自デザインを許可する。その場合、orchestratorの作業報告書へ該当箇所、調査候補、不採用理由、独自実装内容を記載する。
- `frontend/bindings`は手編集せず、直接importは`src/shared/api/wails`だけに限定する。
- `app/pages → widgets/features/entities/shared`の依存方向を守り、必要になるまで階層を作らない。
- React ComponentとReact Hookのファイル名はPascalCaseにする。Hook関数名はlower camel caseにする。`main.tsx`などのentry pointは対象外とする。
- Wails生成DTOのnullable collectionは`shared/api/wails`で正規化する。
- `RuntimeError.cause`は`shared/api/wails`で承認済み`AppErrorCause`へ検証・変換する。
- import境界を変更した場合はaliasと相対pathの両方がOxlintで拒否されることを確認する。
- コードコメントは必要な場合だけ日本語で書く。directive、型参照、自動生成コメントは変更しない。

完了前に`npm --prefix frontend run typecheck`、`lint`、`fmt:check`、`test`、`build`を実行する。E2E・Playwright設定を変更した場合は`task e2e`も実行し、Design System例外の有無を報告する。
