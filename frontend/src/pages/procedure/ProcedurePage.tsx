import { useEffect, useRef, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import { ErrorState } from "@/components/patterns/ErrorState";
import { LoadingState } from "@/components/patterns/LoadingState";
import type {
  EvidenceDetail,
  EvidenceSummary,
  ProcedureDocument,
  ProcedureView,
} from "@/shared/api/wails/procedure";
import "./ProcedurePage.css";

type Props = {
  loading?: boolean;
  view?: ProcedureView;
  error?: string;
  onReload: () => void;
  onSave: (
    document: ProcedureDocument,
    expectedRevision: number,
    operationId: string,
  ) => Promise<void>;
  onRequestRevision: (text: string, expectedRevision: number, operationId: string) => Promise<void>;
  onCancelRevision: (operationId: string) => Promise<void>;
  onRespondElicitation: (
    id: string,
    action: string,
    content: string,
    operationId: string,
  ) => Promise<void>;
  onComplete: (expectedRevision: number, operationId: string) => Promise<void>;
  onExport: (format: "markdown" | "pdf", revision: number, operationId: string) => Promise<boolean>;
  onLoadEvidence: (evidenceId: string, cursor?: string) => Promise<EvidenceDetail>;
  onLoadPreviousConversation?: (cursor: number) => Promise<ProcedureView["conversation"]>;
};

export function ProcedurePage(props: Props) {
  const key = props.view
    ? `${props.view.project.projectId}:${props.view.procedure.procedureId}`
    : "empty";
  return <ProcedureContent key={key} {...props} />;
}

function ProcedureContent({
  loading = false,
  view,
  error,
  onReload,
  onSave,
  onRequestRevision,
  onCancelRevision,
  onRespondElicitation,
  onComplete,
  onExport,
  onLoadEvidence,
  onLoadPreviousConversation,
}: Props) {
  const [editing, setEditing] = useState<ProcedureDocument>();
  const [dirty, setDirty] = useState(false);
  const [editingRevision, setEditingRevision] = useState<number>();
  const [request, setRequest] = useState("");
  const [elicitationContent, setElicitationContent] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState("");
  const [feedback, setFeedback] = useState("");
  const [evidence, setEvidence] = useState<EvidenceDetail>();
  const [evidenceError, setEvidenceError] = useState("");
  const [evidenceBusy, setEvidenceBusy] = useState(false);
  const [conversationState, setConversationState] = useState<{
    source?: ProcedureView["conversation"];
    items: ProcedureView["conversation"]["items"];
    cursor?: number;
    busy: boolean;
    error: string;
  }>({
    source: view?.conversation,
    items: [],
    cursor: view?.conversation.previousCursor,
    busy: false,
    error: "",
  });
  const currentConversation =
    conversationState.source === view?.conversation
      ? conversationState
      : {
          source: view?.conversation,
          items: [],
          cursor: view?.conversation.previousCursor,
          busy: false,
          error: "",
        };
  const dialog = useRef<HTMLDialogElement>(null);
  const openedBy = useRef<HTMLElement | null>(null);
  const pendingOperation = useRef<{ key: string; id: string } | undefined>(undefined);
  const active = useRef(true);
  const evidenceRequest = useRef(0);
  const evidencePending = useRef<number | undefined>(undefined);

  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);

  const document = editing ?? view?.procedure.document;

  async function run(
    key: string,
    action: (id: string) => Promise<void | boolean>,
    success: string,
  ) {
    const id =
      pendingOperation.current?.key === key ? pendingOperation.current.id : crypto.randomUUID();
    pendingOperation.current = { key, id };
    setBusy(key);
    setFeedback("");
    try {
      const result = await action(id);
      if (!active.current) return;
      if (result === false) {
        pendingOperation.current = undefined;
        return;
      }
      pendingOperation.current = undefined;
      setFeedback(success);
      if (key === "save") {
        setDirty(false);
        setEditing(undefined);
        setEditingRevision(undefined);
      }
      if (key === "revision") setRequest("");
      onReload();
    } catch (cause) {
      if (active.current)
        setFeedback(
          (cause instanceof Error ? cause.message : "操作に失敗しました。") +
            (key === "save"
              ? " 入力は保持されています。再取得して変更内容を確認してください。"
              : " 再試行できます。"),
        );
    } finally {
      if (active.current) setBusy("");
    }
  }

  function change(value: ProcedureDocument) {
    if (!dirty) setEditingRevision(view?.procedure.revision);
    setEditing(value);
    setDirty(true);
    pendingOperation.current = undefined;
  }

  async function openEvidence(summary: EvidenceSummary) {
    const sequence = ++evidenceRequest.current;
    openedBy.current = window.document.activeElement as HTMLElement;
    setEvidence(undefined);
    setEvidenceError("");
    dialog.current?.showModal();
    setEvidenceBusy(true);
    try {
      const detail = await onLoadEvidence(summary.evidenceId);
      if (active.current && sequence === evidenceRequest.current) setEvidence(detail);
    } catch (cause) {
      if (active.current && sequence === evidenceRequest.current)
        setEvidenceError(cause instanceof Error ? cause.message : "証跡を取得できませんでした。");
    } finally {
      if (active.current && sequence === evidenceRequest.current) setEvidenceBusy(false);
    }
  }

  async function loadMoreEvidence() {
    const current = evidence;
    const cursor = current?.textPage?.nextCursor;
    const sequence = evidenceRequest.current;
    if (!current || !cursor || evidencePending.current === sequence) return;
    evidencePending.current = sequence;
    setEvidenceBusy(true);
    setEvidenceError("");
    try {
      const next = await onLoadEvidence(current.summary.evidenceId, cursor);
      if (active.current && sequence === evidenceRequest.current)
        setEvidence((previous) =>
          previous?.summary.evidenceId === current.summary.evidenceId
            ? {
                ...next,
                textPage: next.textPage
                  ? {
                      ...next.textPage,
                      content: previous.textPage!.content + next.textPage.content,
                    }
                  : previous.textPage,
              }
            : previous,
        );
    } catch (cause) {
      if (active.current && sequence === evidenceRequest.current)
        setEvidenceError(cause instanceof Error ? cause.message : "続きを取得できませんでした。");
    } finally {
      if (evidencePending.current === sequence) evidencePending.current = undefined;
      if (active.current && sequence === evidenceRequest.current) setEvidenceBusy(false);
    }
  }

  async function loadPreviousConversation() {
    if (
      currentConversation.cursor === undefined ||
      !onLoadPreviousConversation ||
      currentConversation.busy
    )
      return;
    const source = view?.conversation;
    setConversationState({ ...currentConversation, busy: true, error: "" });
    try {
      const page = await onLoadPreviousConversation(currentConversation.cursor);
      if (!active.current) return;
      setConversationState((state) =>
        state.source === source
          ? {
              ...state,
              items: [...page.items, ...state.items],
              cursor: page.hasPrevious ? page.previousCursor : undefined,
            }
          : state,
      );
    } catch (cause) {
      if (active.current)
        setConversationState((state) =>
          state.source === source
            ? {
                ...state,
                error:
                  cause instanceof Error ? cause.message : "過去の会話を取得できませんでした。",
              }
            : state,
        );
    } finally {
      if (active.current)
        setConversationState((state) =>
          state.source === source ? { ...state, busy: false } : state,
        );
    }
  }

  const procedure = view?.procedure;
  const editable = procedure?.status === "draft";
  const blocked = view?.integrity.issues.some((issue) => issue.severity === "blocking");

  return (
    <main className="procedure-page" aria-labelledby="procedure-title">
      <LoadingState loading={loading} initial label="手順書を読み込んでいます…">
        {error ? (
          <ErrorState description={error} onRetry={onReload} />
        ) : !view || !document ? (
          <div>
            <h1 id="procedure-title">手順書</h1>
            <p>手順書はまだありません。</p>
            <Button onClick={onReload}>再取得</Button>
          </div>
        ) : (
          <>
            <header className="procedure-header">
              <div>
                <p className="procedure-eyebrow">{view.project.name} · 手順書</p>
                <h1 id="procedure-title">{document.title || "無題の手順書"}</h1>
                <p>
                  版 {procedure!.revisionNumber} ·{" "}
                  {procedure!.status === "completed"
                    ? "完成"
                    : procedure!.status === "draft"
                      ? "編集中"
                      : "処理中"}
                </p>
              </div>
              <Button variant="outline" onClick={onReload}>
                再取得
              </Button>
            </header>
            <div role="status" aria-live="polite">
              {feedback}
              {dirty && " 未保存の変更があります。"}
            </div>
            {view.integrity.issues.length > 0 && (
              <Alert variant={blocked ? "destructive" : "warning"}>
                <AlertTitle>整合性: {view.integrity.status}</AlertTitle>
                <AlertDescription>
                  <ul>
                    {view.integrity.issues.map((issue, index) => (
                      <li key={`${issue.code}-${index}`}>{issue.message}</li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            )}
            <div className="procedure-layout">
              <section aria-label="AIへの修正依頼" className="procedure-panel">
                <h2>AIに修正を依頼</h2>
                <div className="procedure-conversation">
                  {currentConversation.cursor !== undefined && onLoadPreviousConversation && (
                    <Button
                      variant="outline"
                      disabled={currentConversation.busy}
                      onClick={() => void loadPreviousConversation()}
                    >
                      過去の会話を表示
                    </Button>
                  )}
                  {currentConversation.busy && <p role="status">過去の会話を読み込んでいます…</p>}
                  {currentConversation.error && <p role="alert">{currentConversation.error}</p>}
                  {[...currentConversation.items, ...view.conversation.items].map((item) => (
                    <p key={item.messageId}>
                      <strong>{item.role === "user" ? "あなた" : "AI"}</strong>{" "}
                      {item.content
                        .filter((part) => part.type === "text")
                        .map((part) => part.text)
                        .join(" ")}
                    </p>
                  ))}
                </div>
                <label htmlFor="procedure-request">修正内容</label>
                <Textarea
                  id="procedure-request"
                  value={request}
                  onChange={(event) => {
                    setRequest(event.target.value);
                    pendingOperation.current = undefined;
                  }}
                  disabled={!editable || !!busy}
                />
                <Button
                  disabled={!editable || !request.trim() || !!busy || dirty}
                  loading={busy === "revision"}
                  onClick={() =>
                    void run(
                      "revision",
                      (id) => onRequestRevision(request, procedure!.revision, id),
                      "修正依頼を受け付けました。",
                    )
                  }
                >
                  修正を依頼
                </Button>
                {view.activeRevision && (
                  <Button
                    variant="outline"
                    disabled={!!busy}
                    loading={busy === "cancel"}
                    onClick={() =>
                      void run("cancel", onCancelRevision, "AIの修正を取り消しました。")
                    }
                  >
                    AIの修正を取り消す
                  </Button>
                )}
                {view.elicitations
                  ?.filter((item) => item.status === "pending")
                  .map((item) => (
                    <section key={item.elicitationRequestId} aria-label="Agentからの確認">
                      <h3>Agentからの確認</h3>
                      <p>{item.message}</p>
                      {item.mode === "form" && (
                        <>
                          <p>JSON形式で回答してください。</p>
                          {item.requestedSchema && (
                            <pre>{JSON.stringify(item.requestedSchema, null, 2)}</pre>
                          )}
                          <label htmlFor={`procedure-elicitation-${item.elicitationRequestId}`}>
                            回答内容
                          </label>
                          <Textarea
                            id={`procedure-elicitation-${item.elicitationRequestId}`}
                            value={elicitationContent[item.elicitationRequestId] ?? "{}"}
                            onChange={(event) =>
                              setElicitationContent((current) => ({
                                ...current,
                                [item.elicitationRequestId]: event.target.value,
                              }))
                            }
                          />
                        </>
                      )}
                      {item.mode === "url" && item.url && /^https?:\/\//.test(item.url) && (
                        <a href={item.url} target="_blank" rel="noreferrer">
                          Agentの確認ページを開く
                        </a>
                      )}
                      {item.mode === "url" && <p>確認後、Agentからの完了通知を待ちます。</p>}
                      {item.mode === "unsupported" && <p>この確認形式には対応していません。</p>}
                      <div className="procedure-actions">
                        {item.mode === "form" && (
                          <Button
                            disabled={!!busy}
                            onClick={() =>
                              void run(
                                `elicitation-accept-${item.elicitationRequestId}`,
                                (id) =>
                                  onRespondElicitation(
                                    item.elicitationRequestId,
                                    "accept",
                                    elicitationContent[item.elicitationRequestId] ?? "{}",
                                    id,
                                  ),
                                "Agentに回答しました。",
                              )
                            }
                          >
                            応答する
                          </Button>
                        )}
                        <Button
                          variant="outline"
                          disabled={!!busy}
                          onClick={() =>
                            void run(
                              `elicitation-decline-${item.elicitationRequestId}`,
                              (id) =>
                                onRespondElicitation(item.elicitationRequestId, "decline", "", id),
                              "Agentの確認を辞退しました。",
                            )
                          }
                        >
                          辞退する
                        </Button>
                      </div>
                    </section>
                  ))}
                {dirty && <p>直接編集を保存してから依頼できます。</p>}
              </section>
              <section aria-label="手順書本文" className="procedure-panel procedure-document">
                <h2>本文</h2>
                <label htmlFor="procedure-name">手順書タイトル</label>
                <Input
                  id="procedure-name"
                  value={document.title}
                  disabled={!editable || !!busy}
                  onChange={(event) => change({ ...document, title: event.target.value })}
                />
                <label htmlFor="procedure-overview">概要</label>
                <Textarea
                  id="procedure-overview"
                  value={document.overview}
                  disabled={!editable || !!busy}
                  onChange={(event) => change({ ...document, overview: event.target.value })}
                />
                <label htmlFor="procedure-prerequisites">前提条件（1行に1件）</label>
                <Textarea
                  id="procedure-prerequisites"
                  value={document.prerequisites.join("\n")}
                  disabled={!editable || !!busy}
                  onChange={(event) =>
                    change({ ...document, prerequisites: event.target.value.split("\n") })
                  }
                />
                <h3>手順</h3>
                {document.steps.map((step, index) => (
                  <fieldset
                    key={step.clientKey}
                    className="procedure-step"
                    disabled={!editable || !!busy}
                  >
                    <legend>手順 {index + 1}</legend>
                    <label htmlFor={`step-title-${step.clientKey}`}>タイトル</label>
                    <Input
                      id={`step-title-${step.clientKey}`}
                      value={step.title}
                      onChange={(event) =>
                        change({
                          ...document,
                          steps: document.steps.map((value, position) =>
                            position === index ? { ...value, title: event.target.value } : value,
                          ),
                        })
                      }
                    />
                    <label htmlFor={`step-description-${step.clientKey}`}>説明</label>
                    <Textarea
                      id={`step-description-${step.clientKey}`}
                      value={step.description}
                      onChange={(event) =>
                        change({
                          ...document,
                          steps: document.steps.map((value, position) =>
                            position === index
                              ? { ...value, description: event.target.value }
                              : value,
                          ),
                        })
                      }
                    />
                    <label htmlFor={`step-command-${step.clientKey}`}>コマンド</label>
                    <Input
                      id={`step-command-${step.clientKey}`}
                      value={step.command ?? ""}
                      onChange={(event) =>
                        change({
                          ...document,
                          steps: document.steps.map((value, position) =>
                            position === index ? { ...value, command: event.target.value } : value,
                          ),
                        })
                      }
                    />
                    <label htmlFor={`step-notes-${step.clientKey}`}>注意事項（1行に1件）</label>
                    <Textarea
                      id={`step-notes-${step.clientKey}`}
                      value={step.notes.join("\n")}
                      onChange={(event) =>
                        change({
                          ...document,
                          steps: document.steps.map((value, position) =>
                            position === index
                              ? { ...value, notes: event.target.value.split("\n") }
                              : value,
                          ),
                        })
                      }
                    />
                    <div className="procedure-actions">
                      <Button
                        variant="outline"
                        disabled={index === 0}
                        onClick={() =>
                          change({ ...document, steps: move(document.steps, index, index - 1) })
                        }
                      >
                        上へ
                      </Button>
                      <Button
                        variant="outline"
                        disabled={index === document.steps.length - 1}
                        onClick={() =>
                          change({ ...document, steps: move(document.steps, index, index + 1) })
                        }
                      >
                        下へ
                      </Button>
                      <Button
                        variant="destructive"
                        onClick={() =>
                          change({
                            ...document,
                            steps: document.steps.filter((_, position) => position !== index),
                          })
                        }
                      >
                        削除
                      </Button>
                    </div>
                    {step.evidenceRefs.map((ref) => (
                      <div key={ref.evidenceId} className="procedure-evidence-ref">
                        <label className="procedure-check">
                          <input
                            type="checkbox"
                            checked={ref.included}
                            onChange={(event) =>
                              change({
                                ...document,
                                steps: document.steps.map((value, position) =>
                                  position === index
                                    ? {
                                        ...value,
                                        evidenceRefs: value.evidenceRefs.map((item) =>
                                          item.evidenceId === ref.evidenceId
                                            ? { ...item, included: event.target.checked }
                                            : item,
                                        ),
                                      }
                                    : value,
                                ),
                              })
                            }
                          />
                          証跡 {ref.evidenceId} を参照する
                        </label>
                        <label htmlFor={`evidence-name-${step.clientKey}-${ref.evidenceId}`}>
                          証跡 {ref.evidenceId} の表示名
                        </label>
                        <Input
                          id={`evidence-name-${step.clientKey}-${ref.evidenceId}`}
                          value={ref.displayName}
                          onChange={(event) =>
                            change({
                              ...document,
                              steps: document.steps.map((value, position) =>
                                position === index
                                  ? {
                                      ...value,
                                      evidenceRefs: value.evidenceRefs.map((item) =>
                                        item.evidenceId === ref.evidenceId
                                          ? { ...item, displayName: event.target.value }
                                          : item,
                                      ),
                                    }
                                  : value,
                              ),
                            })
                          }
                        />
                      </div>
                    ))}
                  </fieldset>
                ))}
                {editable && (
                  <div className="procedure-actions">
                    <Button
                      variant="outline"
                      disabled={!!busy}
                      onClick={() =>
                        change({
                          ...document,
                          steps: [
                            ...document.steps,
                            {
                              clientKey: crypto.randomUUID(),
                              title: "",
                              description: "",
                              notes: [],
                              evidenceRefs: [],
                            },
                          ],
                        })
                      }
                    >
                      手順を追加
                    </Button>
                    <Button
                      disabled={!dirty || !!busy}
                      loading={busy === "save"}
                      onClick={() =>
                        void run(
                          "save",
                          (id) => onSave(document, editingRevision ?? procedure!.revision, id),
                          "保存しました。",
                        )
                      }
                    >
                      保存
                    </Button>
                    <Button
                      variant="outline"
                      disabled={!dirty || !!busy}
                      onClick={() => {
                        setEditing(undefined);
                        setDirty(false);
                        setEditingRevision(undefined);
                        pendingOperation.current = undefined;
                      }}
                    >
                      変更を破棄
                    </Button>
                  </div>
                )}
              </section>
              <aside className="procedure-panel" aria-label="証跡と完成">
                <h2>確認の証跡</h2>
                <p>
                  元の動作チェック: {view.source.checkCount} 項目 · 証跡 {view.source.evidenceCount}{" "}
                  件
                </p>
                {(["ai", "human"] as const).map((actor) => (
                  <section key={actor} aria-label={actor === "ai" ? "AIの証跡" : "人間の証跡"}>
                    <h3>{actor === "ai" ? "AIの証跡" : "人間の証跡"}</h3>
                    {view.evidence[actor].length === 0 ? (
                      <p>ありません。</p>
                    ) : (
                      <ul>
                        {view.evidence[actor].map((item) => (
                          <li key={item.evidenceId}>
                            <Button variant="link" onClick={() => void openEvidence(item)}>
                              {item.displayName}
                            </Button>
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                ))}
                {editable && (
                  <Button
                    disabled={!!busy || dirty || !!blocked}
                    loading={busy === "complete"}
                    onClick={() =>
                      void run(
                        "complete",
                        (id) => onComplete(procedure!.revision, id),
                        "手順書を完成しました。",
                      )
                    }
                  >
                    手順書を完成
                  </Button>
                )}
                {procedure!.status === "completed" && (
                  <div className="procedure-actions">
                    <Button
                      disabled={!!busy}
                      loading={busy === "markdown"}
                      onClick={() =>
                        void run(
                          "markdown",
                          (id) => onExport("markdown", procedure!.revision, id),
                          "Markdownの出力を受け付けました。",
                        )
                      }
                    >
                      Markdownを出力
                    </Button>
                    <Button
                      disabled={!!busy}
                      loading={busy === "pdf"}
                      onClick={() =>
                        void run(
                          "pdf",
                          (id) => onExport("pdf", procedure!.revision, id),
                          "PDFの出力を受け付けました。",
                        )
                      }
                    >
                      PDFを出力
                    </Button>
                  </div>
                )}
              </aside>
            </div>
          </>
        )}
      </LoadingState>
      <dialog
        ref={dialog}
        className="procedure-dialog"
        aria-labelledby="procedure-evidence-title"
        onClose={() => {
          evidenceRequest.current++;
          openedBy.current?.focus();
        }}
      >
        <div className="procedure-actions">
          <h2 id="procedure-evidence-title">{evidence?.summary.displayName ?? "証跡"}</h2>
          <Button variant="ghost" onClick={() => dialog.current?.close()}>
            閉じる
          </Button>
        </div>
        {evidenceBusy && !evidence ? (
          <p role="status">証跡を読み込んでいます…</p>
        ) : evidenceError && !evidence ? (
          <ErrorState description={evidenceError} />
        ) : (
          evidence && (
            <>
              <section>
                <h3>{evidence.source.actor === "ai" ? "AIの証跡" : "人間の証跡"}</h3>
                <p>確認日時: {evidence.summary.createdAt}</p>
                <p>整合性: {evidence.integrity}</p>
                {evidence.source.actor === "ai" && (
                  <>
                    <p>
                      実行コマンド: <code>{evidence.command}</code>
                    </p>
                    <p>終了コード: {evidence.exitCode}</p>
                  </>
                )}
                {evidence.textPage && <pre>{evidence.textPage.content}</pre>}
                {evidence.image && <img src={evidence.image.previewUrl} alt={evidence.image.alt} />}
              </section>
              {evidence.textPage?.nextCursor && (
                <Button
                  variant="outline"
                  disabled={evidenceBusy}
                  onClick={() => void loadMoreEvidence()}
                >
                  続きを読む
                </Button>
              )}
              {evidenceBusy && <p role="status">続きを読み込んでいます…</p>}
              {evidenceError && <p role="alert">{evidenceError}</p>}
            </>
          )
        )}
      </dialog>
    </main>
  );
}

function move<T>(items: T[], from: number, to: number): T[] {
  const next = [...items];
  next.splice(to, 0, ...next.splice(from, 1));
  return next;
}
