import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";

import type { PreparationView } from "@/shared/api/wails/preparation";

import { PreparationPage } from "./PreparationPage";

const noop = () => {};
const snapshot: PreparationView = {
  project: {
    projectId: "project-1",
    name: "手順書作成",
    description: "準備内容を確認します。",
    workspacePath: "/work/project-1",
    revision: 1,
  },
  preparation: {
    purpose: "操作手順を共有する",
    completionCriteria: ["手順を再現できる"],
    intendedUsers: "チームメンバー",
    revision: 1,
  },
  checkPlan: {
    planId: "plan-1",
    revision: 1,
    items: [
      {
        checkId: "check-1",
        sequence: 1,
        title: "手順を確認する",
        instruction: "手順に従って操作する",
        expectedResult: "正常に完了する",
        aiRequired: false,
        humanRequired: true,
        humanEvidenceRequirement: "none",
      },
    ],
  },
  session: {
    sessionId: "session-1",
    state: "ready",
    permissionPolicy: { mode: "ask_every_time", revision: 1 },
    revision: 1,
    modes: { currentModeId: "default", available: [{ modeId: "default", name: "通常" }] },
  },
  conversation: {
    items: [
      {
        messageId: "message-1",
        turnId: "turn-1",
        role: "agent",
        content: [{ text: "目的と完了条件を確認しましょう。" }],
        status: "completed",
      },
    ],
    hasPrevious: false,
  },
  elicitations: [],
  readiness: { canStartExecution: true, blockingReasons: [] },
  changeSequence: 1,
};

const meta = {
  component: PreparationPage,
  title: "Pages/Preparation",
  args: {
    snapshot,
    onRefresh: noop,
    onSelectChat: noop,
    onNewChat: noop,
    onLoadPrevious: noop,
    onSaveBrief: noop,
    onSavePlan: noop,
    onChangeWorkspace: noop,
    onSavePolicy: noop,
    onSendMessage: noop,
    onStart: noop,
    onCancel: noop,
    onRespondElicitation: noop,
    onSetMode: noop,
    onSetConfigOption: noop,
  },
  parameters: { viewport: { defaultViewport: "desktop" } },
} satisfies Meta<typeof PreparationPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = { args: { snapshot: undefined, loading: true } };
export const Empty: Story = {
  args: {
    snapshot: {
      ...snapshot,
      preparation: { purpose: "", completionCriteria: [], intendedUsers: "", revision: 0 },
      checkPlan: { planId: "plan-1", revision: 0, items: [] },
      session: undefined,
      conversation: { items: [], hasPrevious: false },
      readiness: {
        canStartExecution: false,
        blockingReasons: [{ code: "brief", message: "目的を入力してください。" }],
      },
    },
  },
};
export const ReadyToStart: Story = {};
export const PastChat: Story = {
  args: {
    selectedChatId: "session-old",
    snapshot: {
      ...snapshot,
      chats: [
        { sessionId: "session-1", title: "現在の相談", startedAt: "2026-09-26T12:00:00Z" },
        { sessionId: "session-old", title: "前回の相談", startedAt: "2026-09-25T12:00:00Z" },
      ],
    },
  },
};
export const TurnPending: Story = {
  args: {
    snapshot: {
      ...snapshot,
      session: { ...snapshot.session!, state: "busy" },
      conversation: {
        items: [
          ...snapshot.conversation.items,
          {
            messageId: "message-2",
            turnId: "turn-2",
            role: "user",
            content: [{ text: "確認をお願いします。" }],
            status: "streaming",
          },
        ],
        hasPrevious: false,
      },
    },
  },
};
export const BriefEditing: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: "修正" }));
    await userEvent.type(
      within(canvasElement).getByLabelText("手順書の目的"),
      "。新しい条件を追加",
    );
  },
};
export const PlanConflict: Story = {
  args: { error: "別の変更が保存されました。", conflictRevision: 2 },
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: "項目を編集" }));
    await userEvent.type(within(canvasElement).getByLabelText("確認内容"), "（編集中）");
  },
};
export const WorkspaceConfirm: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "変更" }));
    await userEvent.clear(canvas.getByLabelText("workspace path"));
    await userEvent.type(canvas.getByLabelText("workspace path"), "/work/new-project");
    await userEvent.click(canvas.getByRole("button", { name: "変更する" }));
  },
};
export const PermissionOrElicitation: Story = {
  args: {
    snapshot: {
      ...snapshot,
      elicitations: [
        {
          elicitationRequestId: "request-1",
          mode: "form",
          message: "実行範囲を確認してください。",
          status: "pending",
          requestedSchema: { type: "object" },
        },
      ],
    },
  },
};
export const ErrorRetryable: Story = {
  args: { snapshot: undefined, error: "準備内容を取得できませんでした。" },
};
