import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { PreparationPage, type PreparationPageProps } from "./PreparationPage";

afterEach(cleanup);

const snapshot: NonNullable<PreparationPageProps["snapshot"]> = {
  project: {
    projectId: "p1",
    name: "Project",
    description: "",
    workspacePath: "/work",
    revision: 1,
  },
  preparation: {
    purpose: "original",
    completionCriteria: ["works"],
    intendedUsers: "user",
    revision: 1,
  },
  checkPlan: { planId: "plan", revision: 1, items: [] },
  session: {
    sessionId: "s1",
    state: "ready",
    permissionPolicy: { mode: "ask_every_time", revision: 1 },
    revision: 1,
  },
  conversation: { items: [], hasPrevious: false },
  elicitations: [],
  readiness: {
    canStartExecution: false,
    blockingReasons: [{ code: "brief", message: "目的が必要です" }],
  },
  changeSequence: 1,
};

const props: PreparationPageProps = {
  snapshot,
  onRefresh: vi.fn(),
  onSaveBrief: vi.fn(),
  onSavePlan: vi.fn(),
  onChangeWorkspace: vi.fn(),
  onSavePolicy: vi.fn(),
  onSendMessage: vi.fn(),
  onStart: vi.fn(),
  onCancel: vi.fn(),
  onRespondElicitation: vi.fn(),
  onSetMode: vi.fn(),
  onSetConfigOption: vi.fn(),
};

it("競合後の再取得でも編集中のbriefを保持する", () => {
  const view = render(<PreparationPage {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "修正" }));
  fireEvent.change(screen.getByLabelText("手順書の目的"), { target: { value: "my edit" } });
  view.rerender(
    <PreparationPage
      {...props}
      snapshot={{
        ...snapshot,
        preparation: { ...snapshot.preparation, purpose: "remote", revision: 2 },
      }}
      error="競合しました"
      conflictRevision={2}
    />,
  );
  expect((screen.getByLabelText("手順書の目的") as HTMLTextAreaElement).value).toBe("my edit");
  expect(screen.getByText(/現在の revision: 2/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "目的を保存" }));
  expect(props.onSaveBrief).toHaveBeenCalledWith(expect.objectContaining({ purpose: "my edit" }));
});

it("workspace変更に明示確認を要求する", () => {
  render(<PreparationPage {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "変更" }));
  fireEvent.change(screen.getByLabelText("workspace path"), { target: { value: "/new" } });
  fireEvent.click(screen.getByRole("button", { name: "変更する" }));
  expect(props.onChangeWorkspace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "変更を確定" }));
  expect(props.onChangeWorkspace).toHaveBeenCalledWith("/new");
});

it("Agentの設定は作業場所の変更時だけ表示する", () => {
  render(
    <PreparationPage
      {...props}
      snapshot={{
        ...snapshot,
        session: {
          ...snapshot.session!,
          modes: { currentModeId: "default", available: [{ modeId: "default", name: "通常" }] },
          configOptions: [{ id: "speed", name: "速度", currentValue: true, choices: [] }],
        },
      }}
    />,
  );
  expect(screen.queryByText("Agentのモード")).toBeNull();
  expect(screen.queryByText("Agentの設定")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "変更" }));
  expect(screen.getByText("Agentのモード")).toBeTruthy();
  expect(screen.getByText("Agentの設定")).toBeTruthy();
});

it("新規チャットと過去チャットを切り替えられる", () => {
  const onSelectChat = vi.fn();
  const onNewChat = vi.fn();
  render(
    <PreparationPage
      {...props}
      snapshot={{
        ...snapshot,
        chats: [
          { sessionId: "s1", title: "現在の相談", startedAt: "" },
          { sessionId: "old", title: "過去の相談", startedAt: "" },
        ],
      }}
      selectedChatId="old"
      onSelectChat={onSelectChat}
      onNewChat={onNewChat}
    />,
  );
  expect(
    screen.getByText("過去のチャットを表示しています。現在のチャットを選ぶと会話を続けられます。"),
  ).toBeTruthy();
  expect(screen.queryByLabelText("AIに相談する")).toBeNull();
  fireEvent.change(screen.getByLabelText("過去のチャット"), { target: { value: "s1" } });
  expect(onSelectChat).toHaveBeenCalledWith("");
  fireEvent.click(screen.getByRole("button", { name: "新規チャット" }));
  expect(onNewChat).toHaveBeenCalledOnce();
});
