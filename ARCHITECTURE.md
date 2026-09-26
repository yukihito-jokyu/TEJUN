# TEJUNアーキテクチャ

この文書は、TEJUNへ機能を追加するときのファイル配置、責務、依存方向を定める。実ファイルが必要になった時点でディレクトリを作り、将来用の空packageや分類だけの階層は作らない。

## 全体構成

```text
Wails/React → adapter/wails → application → domain
                             ↑
SQLite・外部技術 ─────────── adapter

bootstrapだけが具体実装を組み立てる
```

| 配置 | 責務 | 追加時のルール |
| --- | --- | --- |
| `cmd/tejun` | process entry pointとFrontend assetのembed | 起動処理を増やさず、`bootstrap.Run`へ委譲する |
| `internal/bootstrap` | Wails、DB、Serviceの生成とlifecycle | 具体実装の組立てだけを置き、業務判断を置かない |
| `internal/domain` | 外部技術を知らない業務概念、値、ルール、エラー | 業務領域ごとにpackageを分け、共有が確定したものだけ`shared`へ置く |
| `internal/application` | use caseと、それが必要とする最小port | 機能名のファイルへ処理と利用側interfaceを置く |
| `internal/adapter/wails` | Service、DTO、mapper、Event | Wails固有の入出力変換とapplication呼出しだけを置く |
| `internal/adapter/sqlite` | DB接続、migration、repository | SQLとSQLite固有実装を置き、domainへ漏らさない |
| `frontend/src` | React UI | 後述のFrontend layerに従う |
| `frontend/bindings` | Wails生成物 | 手編集せず、CIで再生成差分を検査する |
| `frontend/e2e` | macOS向けPlaywright E2E | 利用者操作を確認するspecと起動補助だけを置く |
| `build` | Wailsのbuild・dev設定 | 実装コードや環境別の複製設定を置かない |

## Backendの配置

### ファイルを追加する場所

| 追加するもの | 配置 | 命名・分割 |
| --- | --- | --- |
| 業務上の型、値、検証規則 | `internal/domain/<業務領域>/` | package内では概念名をファイル名にする |
| use case | `internal/application/<機能名>.go` | 一つの利用者操作または密接な操作群をまとめる |
| use caseが要求するport | portを利用する`internal/application`の同じファイル | 実装側ではなく利用側で最小interfaceを定義する |
| Wails公開Service | `internal/adapter/wails/<機能名>.go` | 入力検証、DTO変換、application呼出しに限定する |
| Wails DTO・mapper | 対応するServiceと同じファイル | 独立して増えた場合だけ`<機能名>_dto.go`へ分ける |
| 共通Wails error | `internal/adapter/wails/error.go` | 全Serviceの`MarshalError`をここへ集約する |
| AppEvent | `internal/adapter/wails/events/` | event envelopeと登録だけを置く |
| SQLite repository | `internal/adapter/sqlite/<機能名>.go` | 対応するapplication portを満たす実装にする |
| migration | `internal/adapter/sqlite/migration/<連番>_<内容>.sql` | goose v3のUp/Downを一つの変更単位で記述する |
| 依存の生成・接続 | `internal/bootstrap/app.go` | constructorと終了処理だけを追加する |

機能ごとのファイルを各packageの直下へ置き、`handlers`、`services`、`repositories`のように役割名を重ねた中間階層は作らない。ファイルが大きいという理由だけではpackageを分けず、独立した責務または公開範囲を分離する必要が生じた場合にだけ新しいpackageを作る。

### 依存方向

- `domain`はWails、SQLite、環境変数、Frontendを参照しない。
- `application`は`domain`と標準libraryだけを基本とし、WailsやSQLiteの具体型を参照しない。
- `adapter/wails`は`application`を呼び、`adapter/sqlite`を直接呼ばない。
- `adapter/sqlite`はapplication側が要求するportを実装する。
- `bootstrap`だけがadapterの具体実装を生成して接続する。
- `domain/application`から`adapter`へのimportは`architecture_test.go`で禁止する。

SQLite migrationはgoose v3のSQLを`embed.FS`へ同梱する。各接続で`foreign_keys=ON`、`journal_mode=WAL`、`synchronous=FULL`、`busy_timeout=5000`を有効にする。

Wails error causeは`code`、`message`、`fieldErrors`、`retryable`、`causeId`、`currentRevision`を共通契約とする。AppEventは`eventId`、`name`、`emittedAt`、`aggregateType`、`aggregateId`、`changeSequence`、`streamKey`、`streamRevision`、`correlation`、`payload`の10項目を唯一のenvelopeとする。

## Frontendの配置

依存方向は次の一方向とする。右側のlayerから左側のlayerを参照してはならない。

```text
app → pages → widgets → features → entities → shared
```

`src/components`はDesign System Registryが必要な項目を導入するときだけ生成する共有layerとして扱う。`shared`と同様に`app`、`pages`、`widgets`、`features`、`entities`を参照しない。

| 配置 | 置くもの | 置かないもの |
| --- | --- | --- |
| `src/app` | React起動、Router、Provider、global style | 画面固有UI、業務処理 |
| `src/pages/<画面>` | route単位の画面合成 | Wails Binding直接呼出し、汎用UI |
| `src/widgets/<領域>` | 複数画面で使う独立した画面領域 | 単一操作だけの処理、基礎UI |
| `src/features/<操作>` | 利用者が行う操作、その状態とUI | アプリ全体設定、別featureの内部実装 |
| `src/entities/<業務概念>` | 業務概念の型、表示、純粋な変換 | 画面遷移、利用者操作の完結した流れ |
| `src/shared/api/wails` | 生成Bindingの唯一のimport入口、DTO正規化、error変換 | 画面固有処理 |
| `src/shared/lib` | framework非依存の横断的な小さい処理 | feature固有処理 |
| `src/components` | Design System Registryから必要時に導入したUI、Pattern、Character、Icon、ThemeProvider | 手書きの独自部品、将来用の雛形 |

新しい画面は`pages`、利用者操作は`features`、業務概念は`entities`から検討する。複数画面で独立して再利用する領域になった場合だけ`widgets`へ置く。二つ以上の呼出し元があるという理由だけで`shared`へ移さず、業務語彙と上位layerへの依存がなくなった場合だけ共通化する。

UIは`.agents/skills/design-system`のRegistry、Pattern、Foundation、Character、Tokenを優先する。`src/components`を先に作らず、対象画面に必要な項目を選んでRegistryから導入する。Design Systemにないため独自デザインする場合は、orchestrator作業報告書へ調査候補、不採用理由、独自実装内容を記録する。

`frontend/bindings`を直接importできるのは`src/shared/api/wails`だけとする。nullable collectionと`RuntimeError.cause`はこの境界でFrontend向けの型へ変換する。依存方向とBinding直接importは`oxlint.config.ts`で検査する。

### feature内部の配置

```text
features/<操作>/
├── api/                  Wails呼出し、request・response変換
├── model/                操作状態、query、mutation、入力検証、データ取得Hook
├── ui/                   Component
│   └── hooks/            DOM、focus、keyboardなどUI固有Hook
└── lib/                  Reactに依存しないfeature固有の純粋処理
```

Wails呼出しを行うHook、データ取得、mutation、フォームや操作の状態は`model/UseCreateProject.ts`のように置く。DOM、focus、keyboard、resizeなど表示へ密着するHookは`ui/hooks/UseFormFocus.ts`のように置く。Reactに依存しない処理はHookにせず`lib`へ置く。複数featureから利用されるという理由だけでrootの`hooks`や`shared`へ移さず、業務語彙と上位layerへの依存がなくなった場合だけ共通化する。

## テストの配置

| テスト | 配置 | 規則 |
| --- | --- | --- |
| Go unit・contract test | 対象実装と同じpackageの`*_test.go` | ケース名、入力、期待値を持つテーブル駆動とし、`t.Run`で実行する |
| React unit test | 対象ファイルの隣の`*.test.ts`または`*.test.tsx` | VitestとTesting Libraryで表示、操作、境界変換を検証する |
| Storybook story | 対象Componentの隣の`*.stories.tsx` | 既存UIの代表状態だけを置き、検証専用のサンプルUIを作らない |
| architecture test | リポジトリrootの`architecture_test.go` | package間の禁止依存を検査する |
| Frontend E2E | `frontend/e2e/*.spec.ts` | Vite開発サーバーを使い、利用者に観測できる結果を検証する |

E2Eの共通fixtureやhelperは、複数specで同じ責務を共有する時点で`frontend/e2e`直下へ追加する。Issue番号、成功・失敗、URLごとのディレクトリは作らない。

## ファイルを増やす判断

1. 既存の同じ責務のファイルへ追加できるか確認する。
2. 追加内容を所有するlayerと業務領域を決める。
3. 一つのファイルが複数の独立した責務を持つ場合だけ分割する。
4. 新しいpackageや共通helperは、現在の利用箇所と分離理由を説明できる場合だけ作る。
5. 配置または依存方向を変えた場合は、この文書と機械検査を同時に更新する。

生成物、外部toolの設定、テストfixtureを業務実装のpackageへ混在させない。手順だけを説明するREADMEを各directoryへ複製せず、配置規則はこの文書を正とする。

## コード規約

コードコメントは必要な場合だけ日本語で書く。コードから明らかな説明はコメントにせず、言語・toolが要求するdirective、型参照、自動生成コメントはそのまま残す。

Goコードは処理のまとまりと宣言の境界へ空行を置く。具体的な配置は`.golangci.yml`で有効化した`wsl_v5`、`gofumpt`、`goimports`、`golines`の結果を正とする。

React ComponentとReact Hookのファイル名はPascalCaseにする。Componentは`ProjectForm.tsx`、Hookは`UseProjectForm.ts`、対応するテストは`ProjectForm.test.tsx`または`UseProjectForm.test.ts`とする。コード内のComponent名はPascalCase、Hook関数名は`useProjectForm`のようにlower camel caseを維持する。`main.tsx`のようなentry point、設定、生成物、Reactに依存しないTypeScriptは対象外とし、それぞれのtoolまたは既存の同じ責務に合わせる。

## 開発と検証

`task dev`で開発起動し、`task storybook`で既存React UIを確認する。`task fmt`、`task lint`、`task test`、`task e2e`、`task build`を個別に実行でき、`task check`がBinding差分、静的検査、test、production build、Storybook静的build、E2Eをまとめる。Playwrightは自身が起動したViteのport 9245へ接続し、終了時に起動processを片付ける。Wailsとの統合はGo testとproduction buildで検証する。現在のbuild・CI対象はmacOSのみとする。
