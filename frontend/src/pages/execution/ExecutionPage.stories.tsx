import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ExecutionView } from "@/shared/api/wails/execution";
import { ExecutionPage } from "./ExecutionPage";

const noop = () => {};
const success = async () => true;
const side = (required: boolean, checked = false, status = "pending") => ({
  required,
  checked,
  status,
  checkedAt: null,
  failureSummary: "",
});
const view: ExecutionView = {
  project: {
    projectId: "project-1",
    name: "手順書作成",
    description: "",
    workspacePath: "/work/project-1",
    status: "active",
    currentStage: "execution",
    revision: 1,
    createdAt: "2026-09-27T09:00:00Z",
    updatedAt: "2026-09-27T09:00:00Z",
    connection: null,
  },
  execution: {
    executionId: "execution-1",
    status: "ready",
    revision: 1,
    startedAt: "2026-09-27T09:00:00Z",
    completedAt: null,
  },
  session: {
    sessionId: "session-1",
    state: "ready",
    protocolVersion: "1",
    agentName: "Agent",
    agentVersion: "1",
    startedAt: "2026-09-27T09:00:00Z",
    disconnectedAt: null,
    permissionPolicy: { sessionId: "session-1", mode: "ask_every_time", revision: 1 },
    revision: 1,
    changeSequence: 1,
    capabilities: null,
    modes: null,
    configOptions: null,
  },
  activeRun: null,
  activity: null,
  checks: [
    {
      checkId: "check-1",
      sequence: 1,
      title: "画面が開く",
      instruction: "アプリを起動する",
      expectedResult: "開始画面が表示される",
      suggestedCommand: "task dev",
      ai: side(true),
      human: side(true),
      humanEvidenceRequirement: "text_or_image",
      overallStatus: "pending",
      evidence: { ai: [], human: [] },
    },
  ],
  pendingPermissions: [],
  conversation: { items: [], previousCursor: null, hasPrevious: false },
  readiness: {
    canGenerateProcedure: false,
    blockingReasons: [
      {
        code: "incomplete",
        message: "チェックを完了してください。",
        checkId: "check-1",
        evidenceRequirement: "",
      },
    ],
  },
  changeSequence: 1,
};

const meta = {
  component: ExecutionPage,
  title: "Pages/Execution",
  args: {
    view,
    loading: false,
    busy: false,
    error: "",
    onRun: noop,
    onHumanCheck: noop,
    onEvidence: success,
    onPermission: noop,
    onGenerate: noop,
  },
  parameters: { viewport: { defaultViewport: "desktop" } },
} satisfies Meta<typeof ExecutionPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};
export const AIUpdateAnimation: Story = {
  name: "AI更新アニメーション",
  render: function AIUpdateAnimationStory(args) {
    const [current, setCurrent] = useState(view);
    useEffect(() => {
      const timer = window.setInterval(() => {
        setCurrent((previous) => ({
          ...previous,
          checks: previous.checks.map((check) => ({
            ...check,
            ai: check.ai.checked
              ? { ...side(true, false, "failed"), failureSummary: "表示を再確認してください。" }
              : side(true, true, "completed"),
          })),
        }));
      }, 3000);
      return () => window.clearInterval(timer);
    }, []);
    return <ExecutionPage {...args} view={current} />;
  },
};

export const Running: Story = {
  args: {
    view: {
      ...view,
      activeRun: {
        runId: "run-1",
        status: "running",
        targetedCheckIds: ["check-1"],
        startedAt: "2026-09-27T09:01:00Z",
        finishedAt: null,
      },
    },
  },
};
export const Complete: Story = {
  args: {
    view: {
      ...view,
      checks: [
        {
          ...view.checks[0],
          ai: side(true, true, "completed"),
          human: side(true, true, "completed"),
          overallStatus: "completed",
          evidence: {
            ai: [],
            human: [
              {
                evidenceId: "evidence-complete",
                actor: "human",
                kind: "text",
                text: "開始画面の表示を確認した",
                displayName: "開始画面の表示を確認した",
                mimeType: "text/plain",
                size: 16,
                previewUrl: "",
                createdAt: "2026-09-27T09:03:00Z",
              },
            ],
          },
        },
      ],
      readiness: { canGenerateProcedure: true, blockingReasons: [] },
    },
  },
};
export const Permission: Story = {
  args: {
    view: {
      ...view,
      pendingPermissions: [
        {
          permissionRequestId: "permission-1",
          sessionId: "session-1",
          runId: "run-1",
          status: "pending",
          requestedAt: "2026-09-27T09:01:00Z",
          expiresAt: null,
          toolCall: {
            toolCallId: "tool-1",
            title: "テストを実行",
            kind: "execute",
            status: "pending",
            command: "go test ./...",
            locations: null,
            details: null,
          },
          options: [
            { optionId: "allow", name: "許可", kind: "allow" },
            { optionId: "deny", name: "拒否", kind: "deny" },
          ],
        },
      ],
    },
  },
};
export const Evidence: Story = {
  args: {
    view: {
      ...view,
      checks: [
        {
          ...view.checks[0],
          evidence: {
            ai: [
              {
                evidenceId: "evidence-1",
                actor: "ai",
                kind: "image",
                text: "",
                displayName: "開始画面",
                mimeType: "image/png",
                size: 127,
                previewUrl:
                  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEAAAAAkCAIAAAC2bqvFAAAARklEQVR42u3YMQ0AIAwAwcpBIsJwxdSFuSYgocklb+DWj52ndQEAAAAA8Aow5vokAAAAAAAAAAAAAAAAAAAAWwUAAADgbgXuqcGqMfXU6QAAAABJRU5ErkJggg==",
                createdAt: "2026-09-27T09:02:00Z",
              },
            ],
            human: [
              {
                evidenceId: "evidence-2",
                actor: "human",
                kind: "text",
                text: "表示を確認した",
                displayName: "表示を確認した",
                mimeType: "text/plain",
                size: 12,
                previewUrl: "",
                createdAt: "2026-09-27T09:03:00Z",
              },
            ],
          },
        },
      ],
    },
  },
};
export const Error: Story = {
  args: { error: "証跡を保存できませんでした。更新して再試行してください。" },
};

export const ApprovedLayout: Story = {
  args: {
    view: {
      ...view,
      project: { ...view.project, name: "Webプロジェクトの初期セットアップ" },
      checks: [
        {
          ...view.checks[0],
          checkId: "workspace",
          sequence: 1,
          title: "作業ディレクトリを確認する",
          instruction: "プロジェクトのルートを開く",
          expectedResult: "対象の作業場所が表示される",
          ai: side(true, true, "completed"),
          human: side(true, true, "completed"),
          humanEvidenceRequirement: "none",
        },
        {
          ...view.checks[0],
          checkId: "dependencies",
          sequence: 2,
          title: "依存関係をインストールする",
          instruction: "必要なパッケージを導入する",
          expectedResult: "インストールが正常終了する",
          ai: side(true, true, "completed"),
          humanEvidenceRequirement: "image",
        },
        {
          ...view.checks[0],
          checkId: "tests",
          sequence: 3,
          title: "テストを実行して結果を確認する",
          instruction: "テストを実行する",
          expectedResult: "すべてのテストが成功する",
          humanEvidenceRequirement: "text",
        },
      ],
    },
  },
};
