#!/usr/bin/env python3
"""承認前のIssue案をGitHubへ送信せず、登録結果を決定的に模擬する。"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path


def issue_markdown(issue: dict, number: int) -> str:
    labels = ", ".join(f"`{label}`" for label in issue.get("labels", [])) or "なし"
    metadata = [
        f"- 下書きID: `{issue['id']}`",
        f"- 種別: `{issue['type']}`",
        f"- ラベル: {labels}",
        "- 親Issue: なし",
        "- 子Issue: なし",
        "- 登録状態: 模擬登録済み（GitHubへは未送信）",
    ]
    return f"## ISSUE-{number:02d}: {issue['title']}\n\n" + "\n".join(metadata) + "\n\n" + issue["body"].rstrip() + "\n"


def sha256(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--validation", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()

    validation = json.loads(args.validation.read_text(encoding="utf-8"))
    if not validation.get("valid"):
        print("検証済みでないIssue案は模擬登録できません", file=sys.stderr)
        return 1
    if args.output_dir.exists() and any(args.output_dir.iterdir()):
        print("出力先が空ではありません。過去runを上書きしません", file=sys.stderr)
        return 1

    plan_text = args.input.read_text(encoding="utf-8")
    plan = json.loads(plan_text)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    issue_blocks = [issue_markdown(issue, index) for index, issue in enumerate(plan["issues"], start=1)]
    draft_text = "# Issue登録前レビュー（模擬登録）\n\nGitHubへの書き込みは行っていない。\n\n" + "\n---\n\n".join(issue_blocks)
    registration = {
        "simulation": True,
        "github_writes": 0,
        "mode": plan["mode"],
        "source_sha256": sha256(plan_text),
        "registration_order": [
            {
                "draft_id": issue["id"],
                "title": issue["title"],
                "type": issue["type"],
                "labels": issue.get("labels", []),
                "body_sha256": sha256(issue["body"]),
            }
            for issue in plan["issues"]
        ],
    }
    manifest = {
        "schema_version": 1,
        "source": args.input.name,
        "mode": plan["mode"],
        "issue_count": len(plan["issues"]),
        "github_writes": 0,
        "files": ["issue-drafts.md", "issue-manifest.json", "simulated-registration.json"],
    }
    (args.output_dir / "issue-drafts.md").write_text(draft_text, encoding="utf-8")
    (args.output_dir / "issue-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (args.output_dir / "simulated-registration.json").write_text(
        json.dumps(registration, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(f"模擬登録成功: {args.output_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
