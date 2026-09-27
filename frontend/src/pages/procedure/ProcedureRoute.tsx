import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { onAppEvent, onSystemWake, parseAppError } from "@/shared/api/wails";
import {
  cancelProcedureRevision,
  completeProcedure,
  exportCompletedProcedure,
  getProcedure,
  getProcedureEvidence,
  requestProcedureRevision,
  respondToProcedureElicitation,
  saveProcedureDraft,
  type ProcedureView,
} from "@/shared/api/wails/procedure";
import { ProcedurePage } from "./ProcedurePage";

export function ProcedureRoute() {
  const { projectId = "" } = useParams();
  return <ProjectProcedure key={projectId} projectId={projectId} />;
}

function ProjectProcedure({ projectId }: { projectId: string }) {
  const [view, setView] = useState<ProcedureView>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const request = useRef(0);
  const sequence = useRef(0);
  const procedureIdRef = useRef("");
  const refresh = useCallback(async () => {
    const id = ++request.current;
    try {
      const next = await getProcedure(projectId);
      if (id !== request.current) return;
      sequence.current = next.changeSequence;
      procedureIdRef.current = next.procedure.procedureId;
      setView(next);
      setError("");
    } catch (cause) {
      if (id === request.current)
        setError(parseAppError(cause)?.message ?? "手順書を読み込めませんでした");
    } finally {
      if (id === request.current) setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    const currentRequest = request;
    const timer = window.setTimeout(() => void refresh(), 0);
    const event = onAppEvent(
      (item) => {
        if (
          (item.aggregateId === projectId ||
            item.aggregateId === procedureIdRef.current ||
            item.correlation.projectId === projectId) &&
          (item.changeSequence > sequence.current || item.changeSequence === 0)
        )
          void refresh().catch(() => undefined);
      },
      () => void refresh().catch(() => undefined),
    );
    const wake = onSystemWake(() => void refresh().catch(() => undefined));
    const visible = () => {
      if (document.visibilityState === "visible") void refresh().catch(() => undefined);
    };
    document.addEventListener("visibilitychange", visible);
    return () => {
      ++currentRequest.current;
      window.clearTimeout(timer);
      event();
      wake();
      document.removeEventListener("visibilitychange", visible);
    };
  }, [projectId, refresh]);

  const procedureId = view?.procedure.procedureId ?? "";
  return (
    <ProcedurePage
      loading={loading}
      view={view}
      error={error}
      onReload={() => void refresh()}
      onSave={async (document, revision, operationId) => {
        await saveProcedureDraft(procedureId, document, revision, operationId);
      }}
      onRequestRevision={async (text, revision, operationId) => {
        await requestProcedureRevision(procedureId, text, revision, operationId);
      }}
      onCancelRevision={async (operationId) => {
        if (!view?.activeRevision) throw new Error("取消対象の処理がありません");
        await cancelProcedureRevision(
          view.activeRevision.sessionId,
          view.activeRevision.turnId,
          operationId,
        );
      }}
      onRespondElicitation={async (id, action, content, operationId) => {
        await respondToProcedureElicitation(id, action, content, operationId);
      }}
      onComplete={async (revision, operationId) => {
        await completeProcedure(procedureId, revision, operationId);
      }}
      onExport={(format, revision, operationId) =>
        view
          ? exportCompletedProcedure(view, format, revision, operationId)
          : Promise.resolve(false)
      }
      onLoadEvidence={(evidenceId, cursor) => getProcedureEvidence(procedureId, evidenceId, cursor)}
      onLoadPreviousConversation={async (cursor) =>
        (await getProcedure(projectId, cursor)).conversation
      }
    />
  );
}
