import copy
import unittest

from check_plan import validate, validate_trace


def action(event, work, agent):
    return {"event": event, "work": work, "agents": [agent]}


def final(number, outcomes):
    return {
        "event": "final",
        "agents": [f"audit{number}", f"debt{number}"],
        "outcomes": {str(key): value for key, value in outcomes.items()},
    }


class ProtocolTests(unittest.TestCase):
    def setUp(self):
        self.base = [
            {"event": "approve"},
            action("implement", 4, "i1"),
            action("review_pass", 4, "r1"),
        ]

    def test_normal_flow(self):
        trace = self.base + [
            final(1, {4: "pass"}),
            action("e2e", 12, "e1"),
            action("review_pass", 12, "er1"),
        ]
        self.assertEqual(validate_trace(trace)[12], "done")

    def test_return_thresholds(self):
        tests = [
            {
                "name": "review",
                "trace": self.base[:2]
                + [
                    action("review_return", 4, "r1"),
                    action("implement", 4, "i2"),
                    action("review_return", 4, "r2"),
                ],
                "work": 4,
            },
            {
                "name": "final",
                "trace": self.base
                + [
                    final(1, {4: "return"}),
                    action("implement", 4, "i2"),
                    action("review_pass", 4, "r2"),
                    final(2, {4: "return"}),
                ],
                "work": 4,
            },
        ]
        for case in tests:
            with self.subTest(case["name"]):
                self.assertEqual(validate_trace(case["trace"])[case["work"]], "human")

    def test_approval_and_agent_reuse(self):
        with self.assertRaisesRegex(ValueError, "承認"):
            validate_trace(self.base[1:])
        with self.assertRaisesRegex(ValueError, "再利用"):
            validate_trace(self.base[:2] + [action("review_pass", 4, "i1")])

    def test_plan_required_structure(self):
        data = {
            "issue": 1,
            "requirements": [{"id": 1}],
            "tasks": [
                {
                    "id": 1,
                    "status": "未開始",
                    "phase": "調査",
                    "skills": "スキルなし",
                    "depends": [],
                }
            ],
            "scenarios": [
                {
                    "id": 1,
                    "requirements": [1],
                    "initial": "A",
                    "input": "B",
                    "action": "C",
                    "expected": "D",
                    "method": "E",
                }
            ],
        }
        validate(data)
        for key, value in (("skills", "backend-review"), ("depends", [1]), ("id", 2)):
            with self.subTest(key):
                invalid = copy.deepcopy(data)
                invalid["tasks"][0][key] = value
                with self.assertRaises(ValueError):
                    validate(invalid)


if __name__ == "__main__":
    unittest.main()
