import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronRight, FolderOpen, Plus, Search, Sparkles } from "lucide-react";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ErrorState } from "@/components/patterns/ErrorState";
import { LoadingState } from "@/components/patterns/LoadingState";
import { AppHeader } from "@/widgets/app-header/AppHeader";
import {
  ProjectActionDialog,
  type ProjectAction,
} from "@/features/project-actions/ui/ProjectActionDialog";
import {
  listProjects,
  onAppEvent,
  onSystemWake,
  parseAppError,
  type ProjectFilter,
  type ProjectSummary,
} from "@/shared/api/wails";

import { ProjectRow } from "./ProjectRow";
import { labels, statusIcons } from "./ProjectPresentation";

import "./ProjectListPage.css";

const attentionActions: Record<string, string> = {
  human_waiting: "確認を続ける",
  ai_running: "状況を見る",
  error: "エラーを確認",
};

function errorMessage(cause: unknown) {
  return parseAppError(cause)?.message ?? "予期しないエラーが発生しました";
}

export function ProjectListPage() {
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<ProjectFilter>("all");
  const [items, setItems] = useState<ProjectSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [moreLoading, setMoreLoading] = useState(false);
  const [moreError, setMoreError] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [action, setAction] = useState<ProjectAction>();
  const [selected, setSelected] = useState<ProjectSummary>();
  const requestId = useRef(0);
  const latestSequence = useRef(0);
  const snapshotSequence = useRef(0);
  const actionTrigger = useRef<HTMLElement>(null);

  const refresh = useCallback(() => {
    setLoading(true);
    setError("");
    setMoreError("");
    setRefreshKey((value) => value + 1);
  }, []);

  useEffect(() => {
    const unsubscribe = onAppEvent((event) => {
      if (event.changeSequence > latestSequence.current) {
        latestSequence.current = event.changeSequence;
        refresh();
      }
    }, refresh);
    const unsubscribeWake = onSystemWake(refresh);
    const revalidate = () => refresh();
    window.addEventListener("focus", revalidate);
    window.addEventListener("pageshow", revalidate);
    return () => {
      unsubscribe();
      unsubscribeWake();
      window.removeEventListener("focus", revalidate);
      window.removeEventListener("pageshow", revalidate);
    };
  }, [refresh]);

  useEffect(() => {
    const current = ++requestId.current;
    let cancelled = false;
    listProjects(search.trim(), filter)
      .then((result) => {
        if (cancelled || current !== requestId.current) return;
        if (result.changeSequence < latestSequence.current) {
          refresh();
          return;
        }
        snapshotSequence.current = result.changeSequence;
        latestSequence.current = Math.max(latestSequence.current, result.changeSequence);
        setItems(result.items);
        setTotal(result.total);
        setNextCursor(result.nextCursor);
      })
      .catch((cause) => {
        if (cancelled || current !== requestId.current) return;
        setError(errorMessage(cause));
      })
      .finally(() => {
        if (!cancelled && current === requestId.current) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [search, filter, refreshKey, refresh]);

  function openAction(next: ProjectAction, trigger: HTMLElement, project?: ProjectSummary) {
    if (trigger instanceof HTMLButtonElement) actionTrigger.current = trigger;
    setAction(next);
    setSelected(project);
  }

  function closeAction() {
    setAction(undefined);
  }

  async function loadMore() {
    if (!nextCursor || moreLoading) return;
    const current = requestId.current;
    setMoreLoading(true);
    try {
      const result = await listProjects(search.trim(), filter, nextCursor);
      if (current !== requestId.current) return;
      if (result.changeSequence !== snapshotSequence.current) {
        refresh();
        return;
      }
      setItems((previous) => [
        ...previous,
        ...result.items.filter((item) => !previous.some((old) => old.projectId === item.projectId)),
      ]);
      setTotal(result.total);
      setNextCursor(result.nextCursor);
    } catch (cause) {
      if (current === requestId.current) setMoreError(errorMessage(cause));
    } finally {
      if (current === requestId.current) setMoreLoading(false);
    }
  }

  return (
    <main className="project-page">
      <AppHeader title="TEJUN" />
      <div className="project-shell">
        <section className="project-heading">
          <div>
            <p className="project-eyebrow">手順書プロジェクト</p>
            <h1>作業を続ける</h1>
            <p>確認待ちの作業や、完成した手順書をここから開けます。</p>
          </div>
          <Button
            type="button"
            size="lg"
            onClick={(event) => openAction("create", event.currentTarget)}
          >
            <Plus size={18} aria-hidden="true" />
            新しい手順書
          </Button>
        </section>

        {!loading && !error && items.some((item) => item.attentionRank > 0) && (
          <section className="project-attention" aria-labelledby="attention-title">
            <div className="project-section-title">
              <h2 id="attention-title">
                <Sparkles size={22} aria-hidden="true" />
                次に対応すること
              </h2>
              <span className="project-attention-order">優先度順</span>
            </div>
            <div className="project-attention-grid">
              {items
                .filter((item) => item.attentionRank > 0)
                .slice(0, 2)
                .map((item) => {
                  const StatusIcon =
                    statusIcons[item.status as keyof typeof statusIcons] ?? FolderOpen;
                  return (
                    <article key={item.projectId} className="project-card">
                      <div className="project-attention-icon" aria-hidden="true">
                        <StatusIcon size={19} />
                      </div>
                      <div className="project-attention-copy">
                        <span className={`project-status project-attention-status-${item.status}`}>
                          {labels[item.status] ?? item.status}
                        </span>
                        <h3>{item.name}</h3>
                        <p>
                          {item.errorSummary ??
                            (item.attentionReason === labels[item.status]
                              ? item.description
                              : (item.attentionReason ?? item.description))}
                        </p>
                      </div>
                      <Button
                        type="button"
                        variant={
                          item.status === "human_waiting" || item.status === "error"
                            ? "default"
                            : "outline"
                        }
                        onClick={() => void navigate(item.resumeRoute.replace(/^#/, ""))}
                      >
                        {attentionActions[item.status] ?? "作業を続ける"}
                        <ChevronRight size={16} aria-hidden="true" />
                      </Button>
                    </article>
                  );
                })}
            </div>
          </section>
        )}

        <section aria-labelledby="projects-title">
          <div className="project-section-title">
            <h2 id="projects-title">
              <FolderOpen size={22} aria-hidden="true" />
              すべてのプロジェクト
            </h2>
            <span aria-live="polite">{total}件</span>
          </div>
          <div className="project-toolbar">
            <div className="project-search">
              <Search size={18} aria-hidden="true" />
              <Input
                id="project-search"
                type="search"
                aria-label="プロジェクトを検索"
                value={search}
                onChange={(event) => {
                  setLoading(true);
                  setError("");
                  setMoreError("");
                  setSearch(event.target.value);
                }}
              />
            </div>
            <div className="project-filters" role="group" aria-label="状態で絞り込む">
              {(["all", "active", "complete"] as const).map((value) => (
                <Button
                  key={value}
                  type="button"
                  variant={filter === value ? "default" : "outline"}
                  aria-pressed={filter === value}
                  onClick={() => {
                    setLoading(true);
                    setError("");
                    setMoreError("");
                    setFilter(value);
                  }}
                >
                  {{ all: "すべて", active: "作業中", complete: "完成" }[value]}
                </Button>
              ))}
            </div>
          </div>
          <LoadingState loading={loading} initial label="プロジェクトを読み込み中">
            {error ? (
              <ErrorState description={error} onRetry={refresh} />
            ) : items.length === 0 ? (
              <div className="project-empty" role="status">
                <h3>
                  {search || filter !== "all"
                    ? "条件に一致するプロジェクトがありません"
                    : "まだプロジェクトがありません"}
                </h3>
                <p>
                  {search || filter !== "all"
                    ? "検索語または絞り込み条件を変更してください。"
                    : "新しい手順書から始めましょう。"}
                </p>
                {search || filter !== "all" ? (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setLoading(true);
                      setError("");
                      setMoreError("");
                      setSearch("");
                      setFilter("all");
                    }}
                  >
                    条件を解除
                  </Button>
                ) : (
                  <Button
                    type="button"
                    onClick={(event) => openAction("create", event.currentTarget)}
                  >
                    <Plus size={18} aria-hidden="true" />
                    新しい手順書
                  </Button>
                )}
              </div>
            ) : (
              <div className="project-list">
                {items.map((item) => (
                  <ProjectRow
                    key={item.projectId}
                    item={item}
                    onOpen={() => void navigate(item.resumeRoute.replace(/^#/, ""))}
                    onAction={(next, trigger) => openAction(next, trigger, item)}
                  />
                ))}
                {nextCursor && (
                  <>
                    {moreError && <p role="alert">{moreError}</p>}
                    <Button
                      type="button"
                      variant="outline"
                      loading={moreLoading}
                      onClick={() => {
                        setMoreError("");
                        loadMore().catch(() => undefined);
                      }}
                    >
                      {moreError ? "再試行する" : "さらに読み込む"}
                    </Button>
                  </>
                )}
              </div>
            )}
          </LoadingState>
        </section>
        {notice && (
          <p role="status" className="project-notice">
            {notice}{" "}
            <Button type="button" variant="ghost" onClick={() => setNotice("")}>
              閉じる
            </Button>
          </p>
        )}
      </div>

      {action && (
        <ProjectActionDialog
          key={action + (selected?.projectId ?? "")}
          action={action}
          selected={selected}
          actionTrigger={actionTrigger}
          onClose={closeAction}
          onNotice={setNotice}
          refresh={refresh}
        />
      )}
    </main>
  );
}
