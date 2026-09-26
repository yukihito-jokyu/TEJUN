import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";

import { getPreparation, type PreparationView } from "@/shared/api/wails/preparation";
import { onAppEvent } from "@/shared/api/wails";
import { PreparationRoute } from "./PreparationRoute";

vi.mock("@/shared/api/wails", () => ({ onAppEvent: vi.fn(), parseAppError: vi.fn(() => null) }));
vi.mock("@/shared/api/wails/preparation", () => ({ getPreparation: vi.fn() }));
afterEach(cleanup);

const view = (name: string): PreparationView => ({
  project: { projectId: "p1", name, description: "", workspacePath: "/work", revision: 1 },
  preparation: { purpose: "", completionCriteria: [], intendedUsers: "", revision: 1 },
  checkPlan: { planId: "plan", revision: 1, items: [] },
  conversation: { items: [], hasPrevious: false },
  elicitations: [],
  readiness: { canStartExecution: false, blockingReasons: [] },
  changeSequence: 1,
});

it("遅い再取得応答で新しいEvent後のsnapshotを戻さない", async () => {
  let listener = () => {};
  vi.mocked(onAppEvent).mockImplementation((callback) => {
    listener = () =>
      callback({
        eventId: "event",
        name: "preparation.changed",
        emittedAt: "2026-09-26T00:00:00Z",
        aggregateType: "preparation",
        aggregateId: "p1",
        changeSequence: 2,
        streamKey: "preparation:p1",
        streamRevision: 2,
        correlation: { projectId: "p1" },
        payload: {},
      });
    return vi.fn();
  });
  let resolveOld!: (value: PreparationView) => void;
  const old = new Promise<PreparationView>((resolve) => {
    resolveOld = resolve;
  });
  vi.mocked(getPreparation)
    .mockResolvedValueOnce(view("最初"))
    .mockReturnValueOnce(old)
    .mockResolvedValueOnce(view("最新"));
  render(
    <MemoryRouter initialEntries={["/projects/p1/prepare"]}>
      <Routes>
        <Route path="/projects/:projectId/prepare" element={<PreparationRoute />} />
      </Routes>
    </MemoryRouter>,
  );
  expect(await screen.findByText("最初 / 準備")).toBeTruthy();
  act(() => {
    listener();
    listener();
  });
  expect(await screen.findByText("最新 / 準備")).toBeTruthy();
  await act(async () => {
    resolveOld(view("古い"));
    await old;
  });
  await waitFor(() => expect(screen.getByText("最新 / 準備")).toBeTruthy());
});
