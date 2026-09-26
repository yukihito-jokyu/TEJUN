import { useLayoutEffect, useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { Call, getTransport, setTransport } from "@wailsio/runtime";
import type { Meta, StoryObj } from "@storybook/react-vite";

import type { ProjectSummary } from "@/shared/api/wails";

import { ProjectListPage } from "./ProjectListPage";

const project: ProjectSummary = {
  projectId: "human",
  name: "Webプロジェクトの初期セットアップ",
  description: "初めて起動するための手順",
  workspacePath: "/Users/demo/project",
  status: "human_waiting",
  currentStage: "verification",
  progress: { completed: 3, total: 5 },
  attentionRank: 2,
  attentionReason: "人間の確認が必要です",
  updatedAt: "2026-09-26T09:00:00Z",
  completedAt: null,
  resumeRoute: "#/projects/human/check",
  currentProcedureId: null,
  connectionState: "connected",
  errorSummary: null,
  revision: 1,
};

const projects: ProjectSummary[] = [
  project,
  {
    ...project,
    projectId: "error",
    name: "E2Eテストの導入",
    status: "error",
    attentionRank: 1,
    attentionReason: "再接続が必要です",
    connectionState: "disconnected",
    errorSummary: "ACP接続が切れました",
  },
  {
    ...project,
    projectId: "ai",
    name: "リリース前の動作確認",
    status: "ai_running",
    attentionRank: 0,
    attentionReason: "",
    progress: { completed: 4, total: 7 },
  },
  {
    ...project,
    projectId: "complete",
    name: "開発環境の構築",
    status: "completed",
    attentionRank: 0,
    attentionReason: "",
    completedAt: "2026-09-25T09:00:00Z",
    currentProcedureId: "procedure-complete",
    currentProcedureRevision: 3,
  },
  {
    ...project,
    projectId: "editing",
    name: "ローカルDBの準備",
    status: "procedure_editing",
    attentionRank: 0,
    attentionReason: "",
  },
];

type Scenario =
  | "loading"
  | "empty"
  | "projects"
  | "error"
  | "actionPending"
  | "exportNew"
  | "exportOverwrite"
  | "exportError";

function ProjectStory({ scenario }: { scenario: Scenario }) {
  const [ready, setReady] = useState(false);

  useLayoutEffect(() => {
    const previous = getTransport();
    setTransport({
      call: async (objectID: number, method: number, _windowName: string, args: unknown) => {
        if (objectID === 5 && method === 5 && scenario.startsWith("export")) {
          return "/Users/demo/手順書.pdf";
        }
        if (objectID !== 0 || method !== 0 || !args || typeof args !== "object") {
          throw new Error("Storyで未対応のWails呼出しです");
        }
        const request = args as { methodID?: number; args?: unknown[] };
        if (request.methodID === 199265905 && scenario.startsWith("export")) {
          return {
            destination: {
              absolutePath: "/Users/demo/手順書.pdf",
              resolvedPath: "/Users/demo/手順書.pdf",
              verifiedRootId: "story-root",
            },
            destinationDisplayName: "手順書.pdf",
            overwriteRequired: scenario !== "exportNew",
            overwriteIdentity:
              scenario === "exportNew"
                ? null
                : { size: 2048, sha256: "a".repeat(64), device: 1, inode: 2 },
          };
        }
        if (request.methodID === 4197448530 && scenario === "exportError") {
          const error = new Call.RuntimeError("export failed");
          error.cause = {
            code: "destination_changed",
            message: "保存先が変更されました。保存先を選び直してください",
            retryable: true,
          };
          throw error;
        }
        if (request.methodID !== 2753046913) {
          if (scenario === "actionPending") return new Promise<never>(() => undefined);
          throw new Error("Storyで未対応の操作です");
        }
        if (scenario === "loading") return new Promise<never>(() => undefined);
        if (scenario === "error") throw new Error("保存済みプロジェクトを読み込めませんでした");
        const query = request.args?.[0] as
          | { search?: string; statuses?: string[]; cursor?: string }
          | undefined;
        const matches =
          scenario === "empty"
            ? []
            : projects.filter(
                (item) =>
                  (!query?.search || item.name.includes(query.search)) &&
                  (!query?.statuses?.length || query.statuses.includes(item.status)),
              );
        const start = query?.cursor ? 4 : 0;
        return {
          items: matches.slice(start, start + 4),
          total: matches.length,
          nextCursor: start + 4 < matches.length ? "more" : null,
          generatedAt: "2026-09-26T09:00:00Z",
          changeSequence: 1,
        };
      },
    });
    let active = true;
    queueMicrotask(() => {
      if (active) setReady(true);
    });
    return () => {
      active = false;
      setTransport(previous);
    };
  }, [scenario]);

  return ready ? (
    <MemoryRouter>
      <ProjectListPage />
    </MemoryRouter>
  ) : null;
}

const meta = {
  component: ProjectListPage,
  title: "Pages/Projects",
  parameters: { layout: "fullscreen", viewport: { defaultViewport: "desktop" } },
} satisfies Meta<typeof ProjectListPage>;

export default meta;
type Story = StoryObj<typeof meta>;

function waitForButton(root: ParentNode, selector: string) {
  return new Promise<HTMLButtonElement>((resolve) => {
    const find = () => {
      const target = root.querySelector<HTMLButtonElement>(selector);
      if (target) {
        observer.disconnect();
        resolve(target);
      }
    };
    const observer = new MutationObserver(find);
    observer.observe(root, { childList: true, subtree: true });
    find();
  });
}

async function openCompletedAction(canvasElement: HTMLElement, label: string) {
  await waitForButton(canvasElement, ".project-row:nth-child(4) .project-row-actions");
  const buttons = canvasElement.querySelectorAll<HTMLButtonElement>(
    ".project-row:nth-child(4) .project-row-actions button",
  );
  const button = [...buttons].find((item) => item.textContent?.trim() === label);
  if (!button) throw new Error(`${label}ボタンが見つかりません`);
  button.click();
}

async function chooseStoryDestination(canvasElement: HTMLElement) {
  await openCompletedAction(canvasElement, "出力");
  (await waitForButton(document, "[role='dialog'] button:nth-of-type(1)")).click();
  await waitForButton(document, "[role='dialog'] [role='status']");
}

export const Loading: Story = { render: () => <ProjectStory scenario="loading" /> };
export const Empty: Story = {
  render: () => <ProjectStory scenario="empty" />,
  play: async ({ canvasElement }) => {
    (await waitForButton(canvasElement, ".project-filters button:last-child")).click();
  },
};
export const WithProjects: Story = { render: () => <ProjectStory scenario="projects" /> };
export const WithProjectsNarrow: Story = {
  render: () => <ProjectStory scenario="projects" />,
  parameters: { viewport: { defaultViewport: "mobile1" } },
};
export const ErrorRetryable: Story = { render: () => <ProjectStory scenario="error" /> };
export const CreateDialog: Story = {
  render: () => <ProjectStory scenario="projects" />,
  play: async ({ canvasElement }) => {
    (await waitForButton(canvasElement, ".project-heading > button")).click();
    await waitForButton(document, "[role='dialog'] #action-name");
  },
};
export const DuplicateDialog: Story = {
  render: () => <ProjectStory scenario="projects" />,
  play: async ({ canvasElement }) => {
    (
      await waitForButton(
        canvasElement,
        ".project-row:first-child .project-row-actions button:nth-child(2)",
      )
    ).click();
    await waitForButton(document, "[role='dialog'] #action-name");
  },
};
export const ReconnectDialog: Story = {
  render: () => <ProjectStory scenario="projects" />,
  play: async ({ canvasElement }) => {
    (
      await waitForButton(
        canvasElement,
        ".project-row:nth-child(2) .project-row-actions button:nth-child(3)",
      )
    ).click();
    await waitForButton(document, "[role='dialog'] .project-action-buttons button:last-child");
  },
};
export const RevisionDialog: Story = {
  render: () => <ProjectStory scenario="projects" />,
  play: async ({ canvasElement }) => {
    await openCompletedAction(canvasElement, "改訂");
    await waitForButton(document, "[role='dialog'] #action-name");
  },
};
export const ExportNewFile: Story = {
  render: () => <ProjectStory scenario="exportNew" />,
  play: async ({ canvasElement }) => {
    await chooseStoryDestination(canvasElement);
  },
};
export const ExportOverwrite: Story = {
  render: () => <ProjectStory scenario="exportOverwrite" />,
  play: async ({ canvasElement }) => {
    await chooseStoryDestination(canvasElement);
  },
};
export const ExportError: Story = {
  render: () => <ProjectStory scenario="exportError" />,
  play: async ({ canvasElement }) => {
    await chooseStoryDestination(canvasElement);
    (
      await waitForButton(document, "[role='dialog'] .project-action-buttons button:last-child")
    ).click();
    await waitForButton(document, "[role='dialog'] [role='alert']");
  },
};
export const ActionPending: Story = {
  render: () => <ProjectStory scenario="actionPending" />,
  play: async ({ canvasElement }) => {
    (await waitForButton(canvasElement, ".project-row-actions button:last-child")).click();
    (
      await waitForButton(document, "[role='dialog'] .project-action-buttons button:last-child")
    ).click();
  },
};
