import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { onAppEvent, parseAppError } from "@/shared/api/wails";
import {
  cancelAgentOperation,
  changeProjectWorkspace,
  getPreparation,
  respondToPreparationElicitation,
  saveCheckPlan,
  savePreparationBrief,
  saveSessionPermissionPolicy,
  sendPreparationMessage,
  setAgentMode,
  setAgentConfigOption,
  startExecution,
  type PreparationView,
} from "@/shared/api/wails/preparation";
import { PreparationPage } from "./PreparationPage";

type Action = NonNullable<React.ComponentProps<typeof PreparationPage>["busy"]>;

export function PreparationRoute() {
  const { projectId } = useParams();
  const navigate = useNavigate();
  const [snapshot, setSnapshot] = useState<PreparationView>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<Action>();
  const [error, setError] = useState<string>();
  const [conflictRevision, setConflictRevision] = useState<number>();
  const [formVersion, setFormVersion] = useState(0);
  const [selectedChatId, setSelectedChatId] = useState("");
  const generation = useRef(0);
  const latest = useRef<PreparationView>(undefined);
  const operationIds = useRef(new Map<Action, { payload: string; id: string }>());

  const refresh = useCallback(async () => {
    if (!projectId) return;
    const current = ++generation.current;
    try {
      const next = await getPreparation(projectId, undefined, selectedChatId);
      if (current !== generation.current) return;
      latest.current = next;
      setSnapshot(next);
      setLoading(false);
    } catch (cause) {
      if (current !== generation.current) return;
      setError(parseAppError(cause)?.message ?? "準備内容を取得できませんでした");
      setLoading(false);
    }
  }, [projectId, selectedChatId]);

  useEffect(() => {
    queueMicrotask(() => {
      refresh().catch(() => undefined);
    });
    return onAppEvent(() => {
      refresh().catch(() => undefined);
    });
  }, [refresh]);

  const run = useCallback(
    async (
      action: Action,
      payload: unknown,
      operation: (view: PreparationView, operationId: string) => Promise<unknown>,
      resetForm = false,
    ) => {
      const view = latest.current;
      if (!view || busy) return;
      const fingerprint = JSON.stringify([
        payload,
        view.preparation.revision,
        view.checkPlan.revision,
        view.project.revision,
        view.session?.revision,
      ]);
      const previous = operationIds.current.get(action);
      const operationId = previous?.payload === fingerprint ? previous.id : crypto.randomUUID();
      operationIds.current.set(action, { payload: fingerprint, id: operationId });
      setBusy(action);
      setError(undefined);
      setConflictRevision(undefined);
      try {
        await operation(view, operationId);
        operationIds.current.delete(action);
        await refresh();
        if (resetForm) setFormVersion((version) => version + 1);
      } catch (cause) {
        const appError = parseAppError(cause);
        setError(appError?.message ?? "操作を完了できませんでした");
        setConflictRevision(appError?.currentRevision);
        if (appError?.currentRevision !== undefined) refresh().catch(() => undefined);
      } finally {
        setBusy(undefined);
      }
    },
    [busy, refresh],
  );

  if (!projectId) return null;
  return (
    <PreparationPage
      key={`${projectId}-${formVersion}`}
      snapshot={snapshot}
      loading={loading}
      busy={busy}
      error={error}
      conflictRevision={conflictRevision}
      selectedChatId={selectedChatId}
      onSelectChat={setSelectedChatId}
      onLoadPrevious={() => {
        const cursor = latest.current?.conversation.previousCursor;
        if (cursor === undefined) return;
        const requestGeneration = generation.current;
        void getPreparation(projectId, cursor, selectedChatId)
          .then((older) => {
            if (requestGeneration !== generation.current || !latest.current) return;
            latest.current = {
              ...latest.current,
              conversation: {
                ...older.conversation,
                items: [...older.conversation.items, ...latest.current.conversation.items],
              },
            };
            setSnapshot(latest.current);
          })
          .catch((cause) =>
            setError(parseAppError(cause)?.message ?? "過去の会話を取得できませんでした"),
          );
      }}
      onNewChat={() =>
        void run(
          "workspace",
          { newChat: true },
          async (view, operationId) => {
            await changeProjectWorkspace(
              projectId,
              view.project.workspacePath,
              view.project.revision,
              operationId,
            );
            setSelectedChatId("");
          },
          true,
        )
      }
      onRefresh={() => {
        setError(undefined);
        refresh().catch(() => undefined);
      }}
      onSaveBrief={(brief) =>
        void run(
          "brief",
          brief,
          (view, operationId) =>
            savePreparationBrief(
              projectId,
              view.preparation.revision,
              {
                purpose: brief.purpose,
                completionCriteria: brief.completionCriteria,
                intendedUsers: brief.intendedUsers,
              },
              operationId,
            ),
          true,
        )
      }
      onSavePlan={(items) =>
        void run(
          "plan",
          items,
          (view, operationId) =>
            saveCheckPlan(
              projectId,
              view.preparation.revision,
              view.checkPlan.revision,
              items,
              operationId,
            ),
          true,
        )
      }
      onChangeWorkspace={(path) =>
        void run(
          "workspace",
          path,
          (view, operationId) =>
            changeProjectWorkspace(projectId, path, view.project.revision, operationId),
          true,
        )
      }
      onSavePolicy={(mode) =>
        void run(
          "policy",
          mode,
          (view, operationId) => {
            if (!view.session) throw new Error("セッションがありません");
            return saveSessionPermissionPolicy(
              view.session.sessionId,
              view.session.revision,
              mode,
              operationId,
            );
          },
          true,
        )
      }
      onSendMessage={(text) =>
        void run(
          "message",
          text,
          (view, operationId) =>
            sendPreparationMessage(projectId, view.session?.sessionId, text, operationId),
          true,
        )
      }
      onCancel={() =>
        void run(
          "cancel",
          latest.current?.conversation.items.find((item) => item.status === "streaming")?.turnId,
          (view, operationId) => {
            if (!view.session) throw new Error("セッションがありません");
            const turn = [...view.conversation.items]
              .reverse()
              .find((item) => item.status === "streaming");
            if (!turn) throw new Error("取消対象の処理がありません");
            return cancelAgentOperation(view.session.sessionId, turn.turnId, operationId);
          },
        )
      }
      onRespondElicitation={(id, action, content) =>
        void run("elicitation", { id, action, content }, (_, operationId) =>
          respondToPreparationElicitation(id, action, content, operationId),
        )
      }
      onSetMode={(modeId) =>
        void run("configuration", { modeId }, (view, operationId) => {
          if (!view.session) throw new Error("セッションがありません");
          return setAgentMode(view.session.sessionId, view.session.revision, modeId, operationId);
        })
      }
      onSetConfigOption={(configId, value) =>
        void run("configuration", { configId, value }, (view, operationId) => {
          if (!view.session) throw new Error("セッションがありません");
          return setAgentConfigOption(
            view.session.sessionId,
            view.session.revision,
            configId,
            value,
            operationId,
          );
        })
      }
      onStart={() =>
        void run("start", projectId, async (view, operationId) => {
          const result = await startExecution(
            projectId,
            view.preparation.revision,
            view.checkPlan.revision,
            operationId,
          );
          await navigate(result.data.nextRoute.replace(/^#/, ""));
        })
      }
    />
  );
}
