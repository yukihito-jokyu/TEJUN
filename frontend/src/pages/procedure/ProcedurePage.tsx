import { useEffect, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { FoldWelcomeCharacterIcon } from "@/components/icons/FoldWelcomeCharacterIcon";
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
  const [editOpen, setEditOpen] = useState(false);
  const editDialog = useRef<HTMLDialogElement>(null);
  const editOpenedBy = useRef<HTMLElement | null>(null);
  const [dirty, setDirty] = useState(false);
  const [editingRevision, setEditingRevision] = useState<number>();
  const [request, setRequest] = useState("");
  const [elicitationContent, setElicitationContent] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState("");
  const [feedback, setFeedback] = useState("");
  const [evidence, setEvidence] = useState<EvidenceDetail>();

  const [pairedEvidence, setPairedEvidence] = useState<EvidenceDetail[]>([]);
  const [relatedEvidenceFailed, setRelatedEvidenceFailed] = useState(false);
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
  const evidenceSelection = useRef<
    { summary: EvidenceSummary; related: EvidenceSummary[] } | undefined
  >(undefined);
  const evidencePending = useRef<number | undefined>(undefined);

  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);

  const document = view?.procedure.document;
  const editDocument = editing ?? document;

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
        editDialog.current?.close();
        setEditOpen(false);
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

  async function openEvidence(summary: EvidenceSummary, related: EvidenceSummary[] = []) {
    evidenceSelection.current = { summary, related };
    const sequence = ++evidenceRequest.current;
    openedBy.current = window.document.activeElement as HTMLElement;
    setEvidence(undefined);
    setPairedEvidence([]);
    setRelatedEvidenceFailed(false);
    setEvidenceError("");
    dialog.current?.showModal();
    setEvidenceBusy(true);
    try {
      const detail = await onLoadEvidence(summary.evidenceId);
      if (active.current && sequence === evidenceRequest.current) setEvidence(detail);
      const others = await Promise.allSettled(
        related.map((item) => onLoadEvidence(item.evidenceId)),
      );
      if (active.current && sequence === evidenceRequest.current) {
        setRelatedEvidenceFailed(others.some((item) => item.status === "rejected"));
        setPairedEvidence(
          others
            .filter(
              (item): item is PromiseFulfilledResult<EvidenceDetail> => item.status === "fulfilled",
            )
            .map((item) => item.value),
        );
      }
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

  function openEditor() {
    editOpenedBy.current = window.document.activeElement as HTMLElement;
    setEditOpen(true);
    editDialog.current?.showModal();
  }

  function discardEditor() {
    setEditing(undefined);
    setDirty(false);
    setEditingRevision(undefined);
    pendingOperation.current = undefined;
    editDialog.current?.close();
    setEditOpen(false);
  }

  const evidenceById = new Map(
    [...(view?.evidence.ai ?? []), ...(view?.evidence.human ?? [])].map((item) => [
      item.evidenceId,
      item,
    ]),
  );

  return (
    <main className="procedure-page" aria-labelledby="procedure-title">
      <header className="procedure-header">
        <div>
          <strong className="procedure-brand">
            <span aria-hidden="true">
              <FoldWelcomeCharacterIcon size={32} />
            </span>{" "}
            TEJUN
          </strong>
          <p className="procedure-header-caption">{view?.project.name ?? "手順書"} / 手順書</p>
        </div>
        <nav className="procedure-steps" aria-label="作成工程">
          <span>
            <b>1</b> 準備
          </span>
          <ChevronRight size={15} aria-hidden="true" />
          <span>
            <b>2</b> 動作チェック
          </span>
          <ChevronRight size={15} aria-hidden="true" />
          <span className="active" aria-current="step">
            <b>3</b> 手順書
          </span>
        </nav>
      </header>
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
          <div className="procedure-layout">
            <aside aria-label="AIへの修正依頼" className="procedure-chat">
              <div className="procedure-chat-heading">
                <p className="procedure-eyebrow">AIアシスタント</p>
                <h1 id="procedure-title">手順書を仕上げる</h1>
                <span className="procedure-status">
                  {view.activeRevision
                    ? "修正中"
                    : procedure!.status === "completed"
                      ? "完成"
                      : "生成完了"}
                </span>
              </div>
              <div className="procedure-conversation" aria-live="polite">
                <div className="procedure-message procedure-message-ai">
                  <span className="procedure-avatar" aria-hidden="true">
                    AI
                  </span>
                  <p>
                    {view.source.checkCount}件の確認と{view.source.evidenceCount}件の証跡から、
                    {document.steps.length}手順の下書きを作成しました。
                  </p>
                </div>
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
                  <div
                    className={`procedure-message ${item.role === "user" ? "procedure-message-user" : "procedure-message-ai"}`}
                    key={item.messageId}
                  >
                    <span className="procedure-avatar" aria-hidden="true">
                      {item.role === "user" ? "人" : "AI"}
                    </span>
                    <p>
                      <strong className="sr-only">{item.role === "user" ? "あなた" : "AI"}</strong>
                      {item.content
                        .filter((part) => part.type === "text")
                        .map((part) => part.text)
                        .join(" ")}
                    </p>
                  </div>
                ))}
              </div>
              {editable && !view.activeRevision && (
                <div className="procedure-suggestions">
                  <strong>修正を依頼できます</strong>
                  <button
                    type="button"
                    onClick={() => setRequest("初心者向けに注意事項を詳しくして")}
                  >
                    初心者向けに詳しく
                  </button>
                  <button
                    type="button"
                    onClick={() => setRequest("コマンドが失敗した場合の対処を追加して")}
                  >
                    失敗時の対処を追加
                  </button>
                </div>
              )}
              {view.activeRevision && (
                <Button
                  variant="outline"
                  disabled={!!busy}
                  loading={busy === "cancel"}
                  onClick={() => void run("cancel", onCancelRevision, "AIの修正を取り消しました。")}
                >
                  AIの修正を取り消す
                </Button>
              )}
              {view.elicitations
                ?.filter((item) => item.status === "pending")
                .map((item) => (
                  <section
                    key={item.elicitationRequestId}
                    className="procedure-elicitation"
                    aria-label="Agentからの確認"
                  >
                    <h2>Agentからの確認</h2>
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
              <div className="procedure-composer">
                <label htmlFor="procedure-request">手順書の修正をAIへ依頼</label>
                <Textarea
                  id="procedure-request"
                  placeholder="手順書の修正を依頼…"
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
                {dirty && <p>直接編集を保存してから依頼できます。</p>}
              </div>
            </aside>
            <section aria-label="手順書本文" className="procedure-workspace">
              <div className="procedure-toolbar">
                <div>
                  <p className="procedure-eyebrow">
                    {view.project.name} · 手順書 {procedure!.revisionNumber}
                  </p>
                  <div className="procedure-title-row">
                    <h2>{document.title || "無題の手順書"}</h2>
                    <span className="procedure-status">
                      {procedure!.status === "completed" ? "完成" : "下書き"}
                    </span>
                  </div>
                  <p>動作チェック済みの内容から生成</p>
                </div>
                <div className="procedure-actions">
                  {editable && (
                    <Button variant="outline" disabled={!!busy} onClick={openEditor}>
                      直接編集
                    </Button>
                  )}
                  <Button variant="outline" onClick={onReload}>
                    再取得
                  </Button>
                </div>
              </div>
              <div role="status" aria-live="polite">
                {feedback}
                {dirty && " 未保存の変更があります。"}
              </div>
              <Alert
                variant={
                  blocked ? "destructive" : view.integrity.issues.length ? "warning" : "success"
                }
              >
                <AlertTitle>
                  {view.integrity.issues.length
                    ? `動作チェックとの整合性: ${view.integrity.status}`
                    : "動作チェックとの整合性を確認済み"}
                </AlertTitle>
                <AlertDescription>
                  {view.integrity.issues.length ? (
                    <ul>
                      {view.integrity.issues.map((issue, index) => (
                        <li key={`${issue.code}-${index}`}>{issue.message}</li>
                      ))}
                    </ul>
                  ) : (
                    <p>{document.steps.length}手順の内容と証跡を確認しました。</p>
                  )}
                </AlertDescription>
              </Alert>
              <article className="procedure-document">
                <section className="procedure-intro">
                  <h3>この手順書について</h3>
                  <p>{document.overview}</p>
                  <div className="procedure-prerequisites">
                    <strong>前提条件</strong>
                    {document.prerequisites.map((item, index) => (
                      <span key={`${index}-${item}`}>{item}</span>
                    ))}
                  </div>
                </section>
                {document.steps.map((step, index) => (
                  <section className="procedure-read-step" key={step.clientKey}>
                    <div className="procedure-step-heading">
                      <span>{index + 1}</span>
                      <h3>{step.title || `手順 ${index + 1}`}</h3>
                    </div>
                    <p>{step.description}</p>
                    {step.command && (
                      <div className="procedure-command">
                        <code>$ {step.command}</code>
                        <Button
                          variant="ghost"
                          aria-label={`${step.command}をコピー`}
                          onClick={() => void navigator.clipboard?.writeText(step.command ?? "")}
                        >
                          コピー
                        </Button>
                      </div>
                    )}
                    {step.notes.map((note, noteIndex) => (
                      <div className="procedure-note" key={`${noteIndex}-${note}`}>
                        <strong>注意</strong>
                        <p>{note}</p>
                      </div>
                    ))}
                    {step.evidenceRefs
                      .filter((ref) => ref.included)
                      .map((ref) => {
                        const summary = evidenceById.get(ref.evidenceId);
                        return summary ? (
                          <div className="procedure-evidence-link" key={ref.evidenceId}>
                            <span>{ref.displayName || summary.displayName}</span>
                            <Button
                              variant="link"
                              aria-label={`${ref.displayName || summary.displayName}の証跡を見る`}
                              onClick={() =>
                                void openEvidence(
                                  summary,
                                  step.evidenceRefs
                                    .filter(
                                      (item) =>
                                        item.included && item.evidenceId !== summary.evidenceId,
                                    )
                                    .map((item) => evidenceById.get(item.evidenceId))
                                    .filter((item): item is EvidenceSummary => !!item),
                                )
                              }
                            >
                              証跡を見る
                            </Button>
                          </div>
                        ) : null;
                      })}
                  </section>
                ))}
              </article>
              {editable && (
                <p className="procedure-edit-guidance">
                  内容を直すには「直接編集」を選ぶか、左の入力欄から AI に修正を依頼してください。
                </p>
              )}
              <footer className="procedure-completion">
                <div>
                  <strong>
                    {procedure!.status === "completed"
                      ? "手順書が完成しました"
                      : "内容を確認して手順書を完成"}
                  </strong>
                  <p>
                    {procedure!.status === "completed"
                      ? "MarkdownまたはPDFとして出力できます。"
                      : "完成後の変更は改訂版として履歴に残します。"}
                  </p>
                </div>
                <div className="procedure-actions">
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
                    <>
                      <Button
                        variant="outline"
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
                        variant="outline"
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
                    </>
                  )}
                </div>
              </footer>
            </section>
          </div>
        )}
      </LoadingState>
      <dialog
        ref={editDialog}
        className="procedure-dialog procedure-edit-dialog"
        aria-labelledby="procedure-edit-title"
        onClose={() => {
          setEditing(undefined);
          setDirty(false);
          setEditingRevision(undefined);
          pendingOperation.current = undefined;
          setEditOpen(false);
          editOpenedBy.current?.focus();
        }}
      >
        {editOpen && editDocument && editable && (
          <>
            <header className="procedure-dialog-header">
              <div>
                <p className="procedure-eyebrow">手順書を直接編集</p>
                <h2 id="procedure-edit-title">文書全体を編集</h2>
              </div>
              <Button variant="ghost" onClick={discardEditor}>
                閉じる
              </Button>
            </header>
            <div className="procedure-edit-fields">
              <label htmlFor="procedure-name">手順書タイトル</label>
              <Input
                id="procedure-name"
                value={editDocument.title}
                disabled={!!busy}
                onChange={(event) => change({ ...editDocument, title: event.target.value })}
              />
              <label htmlFor="procedure-overview">概要</label>
              <Textarea
                id="procedure-overview"
                value={editDocument.overview}
                disabled={!!busy}
                onChange={(event) => change({ ...editDocument, overview: event.target.value })}
              />
              <label htmlFor="procedure-prerequisites">前提条件（1行に1件）</label>
              <Textarea
                id="procedure-prerequisites"
                value={editDocument.prerequisites.join("\n")}
                disabled={!!busy}
                onChange={(event) =>
                  change({ ...editDocument, prerequisites: event.target.value.split("\n") })
                }
              />
              <h3>作業手順</h3>
              {editDocument.steps.map((step, index) => (
                <fieldset key={step.clientKey} className="procedure-step" disabled={!!busy}>
                  <legend>手順 {index + 1}</legend>
                  <label htmlFor={`step-title-${step.clientKey}`}>タイトル</label>
                  <Input
                    id={`step-title-${step.clientKey}`}
                    value={step.title}
                    onChange={(event) =>
                      change({
                        ...editDocument,
                        steps: editDocument.steps.map((value, position) =>
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
                        ...editDocument,
                        steps: editDocument.steps.map((value, position) =>
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
                        ...editDocument,
                        steps: editDocument.steps.map((value, position) =>
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
                        ...editDocument,
                        steps: editDocument.steps.map((value, position) =>
                          position === index
                            ? { ...value, notes: event.target.value.split("\n") }
                            : value,
                        ),
                      })
                    }
                  />
                  {step.evidenceRefs.map((ref) => (
                    <div key={ref.evidenceId} className="procedure-evidence-ref">
                      <label className="procedure-check">
                        <input
                          type="checkbox"
                          checked={ref.included}
                          onChange={(event) =>
                            change({
                              ...editDocument,
                              steps: editDocument.steps.map((value, position) =>
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
                            ...editDocument,
                            steps: editDocument.steps.map((value, position) =>
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
                  <div className="procedure-actions">
                    <Button
                      variant="outline"
                      disabled={index === 0}
                      onClick={() =>
                        change({
                          ...editDocument,
                          steps: move(editDocument.steps, index, index - 1),
                        })
                      }
                    >
                      上へ
                    </Button>
                    <Button
                      variant="outline"
                      disabled={index === editDocument.steps.length - 1}
                      onClick={() =>
                        change({
                          ...editDocument,
                          steps: move(editDocument.steps, index, index + 1),
                        })
                      }
                    >
                      下へ
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() =>
                        change({
                          ...editDocument,
                          steps: editDocument.steps.filter((_, position) => position !== index),
                        })
                      }
                    >
                      削除
                    </Button>
                  </div>
                </fieldset>
              ))}
              <Button
                variant="outline"
                disabled={!!busy}
                onClick={() =>
                  change({
                    ...editDocument,
                    steps: [
                      ...editDocument.steps,
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
            </div>
            {feedback && <p role="alert">{feedback}</p>}
            <footer className="procedure-actions">
              <Button variant="outline" disabled={!!busy} onClick={discardEditor}>
                変更を破棄
              </Button>
              <Button
                disabled={!dirty || !!busy}
                loading={busy === "save"}
                onClick={() =>
                  void run(
                    "save",
                    (id) => onSave(editDocument, editingRevision ?? procedure!.revision, id),
                    "保存しました。動作チェックとの整合性を再確認します。",
                  )
                }
              >
                保存
              </Button>
            </footer>
          </>
        )}
      </dialog>
      <dialog
        ref={dialog}
        className="procedure-dialog"
        aria-labelledby="procedure-evidence-title"
        onClose={() => {
          evidenceRequest.current++;
          openedBy.current?.focus();
        }}
      >
        <header className="procedure-dialog-header">
          <div>
            <p className="procedure-eyebrow">動作チェックの証跡</p>
            <h2 id="procedure-evidence-title">{evidence?.summary.displayName ?? "証跡"}</h2>
          </div>
          <Button variant="ghost" onClick={() => dialog.current?.close()}>
            閉じる
          </Button>
        </header>
        {evidenceBusy && !evidence ? (
          <p role="status">証跡を読み込んでいます…</p>
        ) : evidenceError && !evidence ? (
          <ErrorState description={evidenceError} />
        ) : (
          evidence && (
            <>
              {(["ai", "human"] as const).map((actor) => {
                const details = [evidence, ...pairedEvidence].filter(
                  (item) => item.source.actor === actor,
                );
                return (
                  <section
                    key={actor}
                    className={`procedure-evidence-owner ${actor === "ai" ? "procedure-evidence-ai" : "procedure-evidence-human"}`}
                    aria-label={actor === "ai" ? "AIの証跡" : "人間の証跡"}
                  >
                    <h3>{actor === "ai" ? "AIの証跡" : "人間の証跡"}</h3>
                    {details.length ? (
                      details.map((detail) => (
                        <div key={detail.summary.evidenceId} className="procedure-evidence-detail">
                          <p>
                            <strong>{detail.summary.displayName}</strong>
                          </p>
                          <p>確認日時: {detail.summary.createdAt}</p>
                          <p>整合性: {detail.integrity}</p>
                          {actor === "ai" && (
                            <>
                              <p>
                                実行コマンド: <code>{detail.command}</code>
                              </p>
                              <p>終了コード: {detail.exitCode}</p>
                            </>
                          )}
                          {detail.textPage && (
                            <div className="procedure-evidence-output">
                              <strong>{actor === "ai" ? "ターミナル出力" : "確認メモ"}</strong>
                              <pre>{detail.textPage.content}</pre>
                            </div>
                          )}
                          {detail.image && (
                            <figure>
                              <img src={detail.image.previewUrl} alt={detail.image.alt} />
                            </figure>
                          )}
                        </div>
                      ))
                    ) : (
                      <p>
                        {relatedEvidenceFailed
                          ? "関連する証跡を取得できませんでした。"
                          : `この項目に${actor === "ai" ? "AI" : "人間"}の証跡はありません。`}
                      </p>
                    )}
                  </section>
                );
              })}
              {relatedEvidenceFailed && (
                <Button
                  variant="outline"
                  onClick={() => {
                    const selection = evidenceSelection.current;
                    if (selection)
                      openEvidence(selection.summary, selection.related).catch((cause) =>
                        setEvidenceError(
                          cause instanceof Error ? cause.message : "証跡を取得できませんでした。",
                        ),
                      );
                  }}
                >
                  証跡を再取得
                </Button>
              )}
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
