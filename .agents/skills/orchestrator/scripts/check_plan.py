import argparse
import json
from pathlib import Path


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(data):
    require(isinstance(data["issue"], int) and data["issue"] > 0, "issueが不正です")
    for key in ("requirements", "tasks", "scenarios"):
        items = data[key]
        require(bool(items), f"{key}が空です")
        require(
            [item["id"] for item in items] == list(range(1, len(items) + 1)),
            f"{key}のIDが連番ではありません",
        )

    tasks = {item["id"]: item for item in data["tasks"]}
    statuses = ("未開始", "実行中", "完了", "差し戻し", "人間判断待ち", "実施不要")
    for task in tasks.values():
        require(task["status"] in statuses, "作業状況が不正です")
        if task["phase"] == "調査":
            require(task["skills"] == "スキルなし", "調査にスキルが指定されています")
        require(
            all(dependency in tasks and dependency != task["id"] for dependency in task["depends"]),
            "先行作業が不正です",
        )

    visited = set()
    active = set()

    def visit(task_id):
        require(task_id not in active, "先行作業が循環しています")
        if task_id in visited:
            return
        active.add(task_id)
        for dependency in tasks[task_id]["depends"]:
            visit(dependency)
        active.remove(task_id)
        visited.add(task_id)

    for task_id in tasks:
        visit(task_id)

    for scenario in data["scenarios"]:
        require(
            scenario["requirements"]
            and all(1 <= requirement <= len(data["requirements"]) for requirement in scenario["requirements"]),
            "受け入れ基準の参照が不正です",
        )
        require(
            all(scenario.get(field) for field in ("initial", "input", "action", "expected", "method")),
            "シナリオの項目が不足しています",
        )


def validate_trace(events):
    approved = False
    seen = set()
    states = {}
    review_counts = {}
    final_counts = {}
    e2e_works = set()

    def agents(event, count):
        identifiers = event.get("agents", [])
        require(len(identifiers) == count, "agent IDが不足しています")
        for identifier in identifiers:
            require(identifier and identifier not in seen, "agent IDが再利用されています")
            seen.add(identifier)

    def returned(work, counts, retry_state):
        counts[work] = counts.get(work, 0) + 1
        states[work] = "human" if counts[work] >= 2 else retry_state

    for event in events:
        kind = event["event"]
        if kind == "approve":
            approved = True
            continue

        require(approved, "承認前に実装されています")
        if kind == "final":
            outcomes = event["outcomes"]
            require(
                outcomes and all(str(work) in outcomes for work in states if work not in e2e_works),
                "最終チェックの対象が不足しています",
            )
            require(
                all(states.get(int(work)) in ("final", "e2e") for work in outcomes),
                "レビュー合格前に最終チェックされています",
            )
            require(all(verdict in ("pass", "return") for verdict in outcomes.values()), "判定が不正です")
            agents(event, 2)
            for key, verdict in outcomes.items():
                work = int(key)
                if verdict == "return":
                    returned(work, final_counts, "fix")
                else:
                    states[work] = "e2e"
            continue

        if kind == "product_failure":
            targets = event["works"]
            require(targets and len(targets) == len(set(targets)), "製品不具合の対象が不正です")
            require(
                any(states[work] == "e2e_review" for work in e2e_works),
                "E2E実行前に製品不具合が記録されています",
            )
            require(
                all(work not in e2e_works and states.get(work) == "e2e" for work in targets),
                "製品不具合の作業が不正です",
            )
            for target in targets:
                states[target] = "fix"
            for work in e2e_works:
                if states[work] == "e2e_review":
                    states[work] = "e2e_retry"
            continue

        work = event["work"]
        state = states.get(work, "ready")
        if kind == "human":
            require(state == "human", "上限到達前に人間へ返されています")
            continue

        require(state != "human", "2回目の差し戻し後に自動作業されています")
        require(kind in ("implement", "review_pass", "review_return", "e2e"), "イベントが不正です")
        agents(event, 1)

        if kind == "implement":
            require(work not in e2e_works and state in ("ready", "fix"), "実装順が不正です")
            states[work] = "review"
        elif kind.startswith("review_"):
            require(state in ("review", "e2e_review"), "レビュー順が不正です")
            if kind == "review_pass":
                states[work] = "done" if work in e2e_works else "final"
            else:
                returned(work, review_counts, "e2e_retry" if work in e2e_works else "fix")
        else:
            products = [state for product, state in states.items() if product not in e2e_works]
            require(products and all(state == "e2e" for state in products), "最終チェック前にE2Eを実行しています")
            require(state in ("ready", "e2e_retry"), "E2E実行順が不正です")
            e2e_works.add(work)
            states[work] = "e2e_review"

    return states


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="構造化計画または実行記録の形式を検査します")
    parser.add_argument("input", type=Path)
    parser.add_argument("--trace", action="store_true", help="実行記録のJSON配列を検査します")
    arguments = parser.parse_args()
    payload = json.loads(arguments.input.read_text())
    if arguments.trace:
        print(json.dumps(validate_trace(payload), ensure_ascii=False))
    else:
        validate(payload)
        print("計画形式は正常です。内容の妥当性は別途確認してください。")
