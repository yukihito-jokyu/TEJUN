import type { Meta, StoryObj } from "@storybook/react-vite";
import type { EvidenceDetail, ProcedureView } from "@/shared/api/wails/procedure";
import { ProcedurePage } from "./ProcedurePage";

const ai = {
  evidenceId: "ai-1",
  actor: "ai" as const,
  kind: "command_output" as const,
  displayName: "AIによる実行結果",
  createdAt: "2026-09-26 10:00",
};
const human = {
  evidenceId: "human-1",
  actor: "human" as const,
  kind: "image" as const,
  displayName: "人間による確認画像",
  createdAt: "2026-09-26 10:10",
};

const view: ProcedureView = {
  project: { projectId: "project-1", name: "サンプルプロジェクト" },
  procedure: {
    procedureId: "procedure-1",
    revision: 3,
    revisionNumber: 2,
    status: "draft",
    document: {
      title: "アプリの起動確認",
      overview: "起動して画面とログを確認します。",
      prerequisites: ["アプリをインストール済み"],
      steps: [
        {
          clientKey: "step-1",
          title: "アプリを起動する",
          description: "起動後に画面を確認します。",
          command: "open TEJUN.app",
          notes: ["起動に失敗した場合はログを確認"],
          evidenceRefs: [
            { evidenceId: ai.evidenceId, displayName: ai.displayName, included: true },
          ],
        },
      ],
    },
  },
  source: { executionId: "execution-1", executionRevision: 1, checkCount: 2, evidenceCount: 2 },
  evidence: { ai: [ai], human: [human] },
  integrity: { status: "valid", issues: [] },
  conversation: {
    items: [
      {
        messageId: "message-1",
        turnId: "turn-1",
        role: "user",
        content: [{ type: "text", text: "手順を整理して" }],
        status: "completed",
      },
      {
        messageId: "message-2",
        turnId: "turn-1",
        role: "agent",
        content: [{ type: "text", text: "手順を整理しました。" }],
        status: "completed",
      },
    ],
    hasPrevious: false,
  },
  changeSequence: 1,
};

const noop = async () => {};
const loadEvidence = async (evidenceId: string, cursor?: string): Promise<EvidenceDetail> => {
  if (evidenceId === ai.evidenceId)
    return {
      summary: ai,
      source: { actor: "ai", runId: "run-1" },
      command: "open TEJUN.app",
      exitCode: 0,
      textPage: cursor
        ? { content: "\n画面とログを確認しました。", truncated: false }
        : { content: "起動しました。", nextCursor: "page-2", truncated: true },
      integrity: "verified",
    };
  if (evidenceId === human.evidenceId)
    return {
      summary: human,
      source: { actor: "human" },
      image: {
        previewUrl:
          "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='480' height='240'%3E%3Crect width='480' height='240' fill='%23ddd'/%3E%3C/svg%3E",
        alt: "起動後の画面",
      },
      integrity: "verified",
    };
  throw new Error("関連付けられていない証跡です。");
};

const meta = {
  component: ProcedurePage,
  title: "Pages/Procedure",
  args: {
    view,
    onReload: () => {},
    onSave: noop,
    onRequestRevision: noop,
    onCancelRevision: noop,
    onRespondElicitation: noop,
    onComplete: noop,
    onExport: async (): Promise<boolean> => true,
    onLoadEvidence: loadEvidence,
    onLoadPreviousConversation: async () => ({ items: [], hasPrevious: false }),
  },
  parameters: { viewport: { defaultViewport: "desktop" } },
} satisfies Meta<typeof ProcedurePage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Draft: Story = {};
export const Completed: Story = {
  args: { view: { ...view, procedure: { ...view.procedure, status: "completed" } } },
};
export const Loading: Story = { args: { loading: true, view: undefined } };
export const Empty: Story = { args: { view: undefined } };
export const RecoveryError: Story = {
  args: { view: undefined, error: "手順書を取得できませんでした。" },
};
export const IntegrityBlocked: Story = {
  args: {
    view: {
      ...view,
      integrity: {
        status: "blocked",
        issues: [
          { code: "missing_evidence", severity: "blocking", message: "参照先の証跡がありません。" },
        ],
      },
    },
  },
};

export const AiOnly: Story = {
  args: { view: { ...view, evidence: { ai: [ai], human: [] } } },
};
export const InvalidEvidence: Story = {
  args: {
    onLoadEvidence: async () => {
      throw new Error("証跡の取得を拒否しました。");
    },
  },
};
export const DirtyAndConflict: Story = {
  args: {
    onSave: async () => {
      throw new Error("別の変更が保存されました。再取得してください。");
    },
  },
};
export const ExportPending: Story = {
  args: {
    view: { ...view, procedure: { ...view.procedure, status: "completed" } },
    onExport: () => new Promise<boolean>(() => {}),
  },
};
export const ExportFailure: Story = {
  args: {
    view: { ...view, procedure: { ...view.procedure, status: "completed" } },
    onExport: async () => {
      throw new Error("出力先の確認に失敗しました。");
    },
  },
};
