# 任意の形式検査

Python 3標準ライブラリだけを使用する。入力ファイルは読み取り専用で、失敗時は非ゼロ終了し、planやlogsを変更しない。

```sh
python3 <skill-root>/scripts/check_plan.py <proposal.json>
python3 <skill-root>/scripts/check_plan.py --trace <events.json>
python3 -B <skill-root>/scripts/test_check_plan.py
```

## 計画入力

issue、requirements、tasks、scenariosを持つJSON objectとする。issueは正の整数とし、3つの配列のidはそれぞれ1からの連番とする。tasksはid、status、phase、skills、dependsを持ち、dependsは作業idの配列とする。scenariosはid、requirements、initial、input、action、expected、methodを持ち、requirementsは受け入れ基準idの配列とする。phaseが調査の場合、skillsは「スキルなし」とする。statusは計画テンプレートの作業状況値とする。

## 記録入力

イベントのJSON配列とする。workは元の実装作業ID、agentsは実際の新規agent IDを使う。通常レビュー不合格はreview_return、audit/debtはfinal、E2Eの製品不具合はproduct_failureで記録する。通常レビュー・最終チェックは各作業で別々に2回目の差し戻しでhuman状態となる。

この検査は記録形式と遷移の整合確認に限る。実際の承認、agent起動、シナリオ網羅、実装品質を保証しない。通常の作業でplanとJSONを二重管理する必要はない。
