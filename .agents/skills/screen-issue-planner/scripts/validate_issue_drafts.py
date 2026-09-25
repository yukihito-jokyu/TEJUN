#!/usr/bin/env python3
"""画面Issueまたは整備Issueの構造制約を検証する。GitHubへは接続しない。"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


SCREEN_SECTIONS = (
    "### 何を構築するか",
    "### ドメイン・配置・ファイル分割",
    "### APIと画面のシーケンス",
    "### 受け入れ基準",
    "### テスト観点と項目",
    "### 検証の完了条件",
)
MAINTENANCE_SECTIONS = ("### 何を構築するか", "### 受け入れ基準", "### 完了後の扱い")
LAYERS = {"frontend", "backend", "persistence", "cross-layer", "operations"}


def add(errors: list[str], condition: bool, message: str) -> None:
    if not condition:
        errors.append(message)


def label_values(labels: list[str], prefix: str) -> list[str]:
    return [label for label in labels if label.startswith(prefix)]


def validate_screen_issue(issue: dict, index: int, errors: list[str]) -> None:
    path = f"issues[{index}]"
    labels = issue.get("labels", [])
    body = issue.get("body", "")
    endpoints = issue.get("api", {}).get("endpoints", [])
    testing = issue.get("testing", [])

    add(errors, issue.get("type") == "feature", f"{path}: 画面Issueのtypeはfeatureである必要があります")
    add(errors, issue.get("parent") is None, f"{path}: 親Issueを設定してはいけません")
    add(errors, issue.get("children") == [], f"{path}: 子Issueを設定してはいけません")
    add(errors, isinstance(issue.get("screen"), str) and issue["screen"], f"{path}: screenが必要です")
    add(errors, len(label_values(labels, "type:")) == 1, f"{path}: typeラベルは1つだけ必要です")
    add(errors, label_values(labels, "type:") == ["type:feature"], f"{path}: typeラベルはtype:featureである必要があります")
    add(errors, len(label_values(labels, "domain:")) == 1, f"{path}: domainラベルは1つだけ必要です")
    add(errors, len(label_values(labels, "screen:")) == 1, f"{path}: screenラベルは1つだけ必要です")
    add(errors, not {"frontend", "backend"}.intersection(labels), f"{path}: レイヤーラベルは付与してはいけません")
    for section in SCREEN_SECTIONS:
        add(errors, section in body, f"{path}: {section} が本文にありません")
    if endpoints:
        add(errors, "```mermaid\nsequenceDiagram" in body, f"{path}: API変更に必要なMermaidのシーケンス図がありません")
    else:
        add(errors, "追加・変更するAPIなし" in body, f"{path}: APIなしの理由を本文に記録する必要があります")
    add(errors, len(testing) >= 5, f"{path}: テスト観点が不足しています")
    for test_index, test in enumerate(testing):
        add(
            errors,
            bool(test.get("focus")) and bool(test.get("level")) and bool(test.get("items")),
            f"{path}.testing[{test_index}]: focus、level、itemsが必要です",
        )


def validate_maintenance_issue(
    issue: dict, index: int, expected_concerns: set[str], enforce_structure_metadata: bool, errors: list[str]
) -> None:
    path = f"issues[{index}]"
    body = issue.get("body", "")
    concern = issue.get("concern", "")
    layer = issue.get("layer")
    depends_on = issue.get("depends_on")
    add(errors, issue.get("type") == "maintenance", f"{path}: 整備Issueのtypeはmaintenanceである必要があります")
    add(errors, issue.get("parent") is None, f"{path}: 親Issueを設定してはいけません")
    add(errors, issue.get("children") == [], f"{path}: 子Issueを設定してはいけません")
    add(errors, concern in expected_concerns, f"{path}: 未整備関心ごとに対応していません")
    add(errors, not issue.get("screen"), f"{path}: 整備Issueにscreenを設定してはいけません")
    if enforce_structure_metadata:
        add(errors, layer in LAYERS, f"{path}: layerは {', '.join(sorted(LAYERS))} のいずれかである必要があります")
        add(errors, isinstance(depends_on, list), f"{path}: depends_onは配列である必要があります")
        add(errors, isinstance(issue.get("physical_impact"), str) and bool(issue["physical_impact"].strip()), f"{path}: physical_impactが必要です")
        if layer == "cross-layer":
            add(
                errors,
                isinstance(issue.get("cross_layer_reason"), str) and bool(issue["cross_layer_reason"].strip()),
                f"{path}: cross-layerにはcross_layer_reasonが必要です",
            )
        if concern.endswith("implementation-structure"):
            add(
                errors,
                isinstance(issue.get("merge_rationale"), str) and bool(issue["merge_rationale"].strip()),
                f"{path}: 実装構造にはmerge_rationaleが必要です",
            )
        if concern.endswith("directory-structure") or concern.endswith("file-boundaries"):
            add(
                errors,
                isinstance(issue.get("separation_rationale"), str) and bool(issue["separation_rationale"].strip()),
                f"{path}: 配置と責務を分離するIssueにはseparation_rationaleが必要です",
            )
    for section in MAINTENANCE_SECTIONS:
        add(errors, section in body, f"{path}: {section} が本文にありません")


def validate(plan: dict) -> list[str]:
    errors: list[str] = []
    schema_version = plan.get("schema_version", 1)
    mode = plan.get("mode")
    assessment = plan.get("repository_assessment", {})
    gaps = assessment.get("missing_concerns", [])
    issues = plan.get("issues", [])

    add(errors, schema_version in {1, 2}, "schema_versionが不正です")
    add(errors, mode in {"screen-implementation", "repository-maintenance"}, "modeが不正です")
    add(errors, isinstance(issues, list) and bool(issues), "issuesが必要です")

    if mode == "screen-implementation":
        add(errors, gaps == [], "整備が必要な場合は画面Issueを作成してはいけません")
        screens = [issue.get("screen") for issue in issues]
        add(errors, all(issue.get("type") == "feature" for issue in issues), "画面分解にはfeature以外を含めてはいけません")
        add(errors, len(screens) == len(set(screens)), "同じ画面のIssueを複数作成してはいけません")
        for index, issue in enumerate(issues):
            validate_screen_issue(issue, index, errors)

    if mode == "repository-maintenance":
        expected_concerns = set(gaps)
        concerns = [issue.get("concern") for issue in issues]
        add(errors, bool(expected_concerns), "整備モードにはmissing_concernsが必要です")
        add(errors, len(concerns) == len(set(concerns)), "1つの整備関心ごとにIssueは1つだけです")
        add(errors, set(concerns) == expected_concerns, "未整備関心ごとと整備Issueが一致していません")
        for index, issue in enumerate(issues):
            validate_maintenance_issue(issue, index, expected_concerns, schema_version >= 2, errors)
        concern_indexes = {issue.get("concern"): index for index, issue in enumerate(issues)}
        if schema_version >= 2:
            for index, issue in enumerate(issues):
                path = f"issues[{index}]"
                for dependency in issue.get("depends_on", []):
                    add(errors, dependency in concern_indexes, f"{path}: depends_onに存在しない関心ごとがあります: {dependency}")
                    if dependency in concern_indexes:
                        add(
                            errors,
                            concern_indexes[dependency] < index,
                            f"{path}: depends_onは登録順で先行する関心ごとを指定する必要があります: {dependency}",
                        )

    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    if args.output.exists():
        print("出力先が既に存在します。過去runのvalidation.jsonを上書きしません", file=sys.stderr)
        return 1

    plan = json.loads(args.input.read_text(encoding="utf-8"))
    errors = validate(plan)
    result = {
        "input": args.input.name,
        "mode": plan.get("mode"),
        "valid": not errors,
        "errors": errors,
        "checked_rules": [
            "整備が必要な場合は画面Issueを作らない",
            "画面ごとに1Issueで親子Issueを作らない",
            "API変更がある画面Issueにはシーケンス図を置く",
            "テスト観点と完了条件を置く",
            "整備Issueは関心ごとに1Issueにする",
            "整備Issueは対象層、依存、実ディレクトリへの影響を記録する",
            "横断Issueと実装構造Issueには統合・横断の根拠を記録する",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if errors:
        print("検証失敗: " + " / ".join(errors), file=sys.stderr)
        return 1
    print(f"検証成功: {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
