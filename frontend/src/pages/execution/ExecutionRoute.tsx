import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { onAppEvent, onSystemWake, parseAppError } from "@/shared/api/wails";
import {
  getExecution,
  runPendingChecks,
  sendExecutionMessage,
  setHumanCheck,
  attachHumanEvidence,
  respondToExecutionPermission,
  generateProcedureDraft,
  type ExecutionView,
} from "@/shared/api/wails/execution";
import { ExecutionPage } from "./ExecutionPage";

export function ExecutionRoute() {
  const { projectId = "" } = useParams();
  return <ProjectExecution key={projectId} projectId={projectId} />;
}

function ProjectExecution({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const [view, setView] = useState<ExecutionView>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [older, setOlder] = useState<ExecutionView["conversation"]>();
  const [loadingOlder, setLoadingOlder] = useState(false);
  const request = useRef(0);
  const sequence = useRef(0);
  const refresh = useCallback(async () => {
    const id = ++request.current;
    try {
      const next = await getExecution(projectId);
      if (id !== request.current) return;
      sequence.current = next.changeSequence;
      setView(next);
    } catch (cause) {
      if (id === request.current)
        setError(parseAppError(cause)?.message ?? "動作チェックを読み込めませんでした");
    } finally {
      if (id === request.current) setLoading(false);
    }
  }, [projectId]);
  useEffect(() => {
    const currentRequest = request;
    const timer = window.setTimeout(() => void refresh().catch(() => undefined), 0);
    const event = onAppEvent(
      (item) => {
        if (
          item.aggregateId === projectId ||
          item.correlation.projectId === projectId ||
          item.aggregateId === view?.execution.executionId ||
          item.aggregateId === view?.session.sessionId
        ) {
          if (item.changeSequence > sequence.current || item.changeSequence === 0)
            void refresh().catch(() => undefined);
        }
      },
      () => void refresh().catch(() => undefined),
    );
    const wake = onSystemWake(() => void refresh().catch(() => undefined));
    const focus = () => {
      if (document.visibilityState === "visible") void refresh().catch(() => undefined);
    };
    document.addEventListener("visibilitychange", focus);
    return () => {
      ++currentRequest.current;
      window.clearTimeout(timer);
      event();
      wake();
      document.removeEventListener("visibilitychange", focus);
    };
  }, [projectId, refresh, view?.execution.executionId, view?.session.sessionId]);
  const activeRunId = view?.activeRun?.runId;
  useEffect(() => {
    if (!activeRunId) return;
    const timer = window.setInterval(() => void refresh().catch(() => undefined), 2000);
    return () => window.clearInterval(timer);
  }, [activeRunId, refresh]);
  const mutate = async (call: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    try {
      await call();
      await refresh();
      return true;
    } catch (cause) {
      setError(
        parseAppError(cause)?.message ?? "操作に失敗しました。内容を確認して再試行してください",
      );
      await refresh();
      return false;
    } finally {
      setBusy(false);
    }
  };
  return (
    <ExecutionPage
      view={
        view
          ? {
              ...view,
              conversation: {
                ...view.conversation,
                items: [...(older?.items ?? []), ...view.conversation.items],
                hasPrevious: older ? older.hasPrevious : view.conversation.hasPrevious,
                previousCursor: older ? older.previousCursor : view.conversation.previousCursor,
              },
            }
          : undefined
      }
      loadingOlder={loadingOlder}
      onLoadPrevious={() => {
        const cursor = older ? older.previousCursor : view?.conversation.previousCursor;
        if (cursor == null || loadingOlder) return;
        setLoadingOlder(true);
        void getExecution(projectId, cursor)
          .then((page) =>
            setOlder((current) => ({
              ...page.conversation,
              items: [...page.conversation.items, ...(current?.items ?? [])],
            })),
          )
          .catch((cause) =>
            setError(parseAppError(cause)?.message ?? "以前の会話を読み込めませんでした"),
          )
          .finally(() => setLoadingOlder(false));
      }}
      loading={loading}
      busy={busy}
      error={error}
      onRun={() =>
        view &&
        void mutate(() => runPendingChecks(view.execution.executionId, view.execution.revision))
      }
      onMessage={(text) =>
        view
          ? mutate(() => sendExecutionMessage(view.execution.executionId, text))
          : Promise.resolve(false)
      }
      onHumanCheck={(checkId, checked) =>
        view &&
        void mutate(() =>
          setHumanCheck(view.execution.executionId, checkId, checked, view.execution.revision),
        )
      }
      onEvidence={(checkId, kind, text, path) =>
        view
          ? mutate(() =>
              attachHumanEvidence(
                view.execution.executionId,
                checkId,
                view.execution.revision,
                kind,
                text,
                path,
              ),
            )
          : Promise.resolve(false)
      }
      onPermission={(sessionId, id, optionId) => {
        void mutate(() => respondToExecutionPermission(sessionId, id, optionId)).catch(
          () => undefined,
        );
      }}
      onGenerate={() => {
        if (!view) return;
        setBusy(true);
        setError("");
        void generateProcedureDraft(view.execution.executionId, view.execution.revision)
          .then((result) => void navigate(result.data.nextRoute.replace(/^#/, "")))
          .catch((cause) =>
            setError(parseAppError(cause)?.message ?? "手順書を生成できませんでした"),
          )
          .finally(() => setBusy(false));
      }}
    />
  );
}
