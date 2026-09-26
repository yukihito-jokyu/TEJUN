import { useRef } from "react";
import { DropdownMenu } from "radix-ui";
import { Archive, Clock3, Copy, FolderOpen, MoreHorizontal, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/Button";
import type { ProjectAction } from "@/features/project-actions/ui/ProjectActionDialog";
import type { ProjectSummary } from "@/shared/api/wails";

import { labels, relativeTime, statusIcons } from "./ProjectPresentation";

export function ProjectRow({
  item,
  onOpen,
  onAction,
}: {
  item: ProjectSummary;
  onOpen: () => void;
  onAction: (action: ProjectAction, trigger: HTMLElement) => void;
}) {
  const trigger = useRef<HTMLElement>(null);
  const StatusIcon = statusIcons[item.status as keyof typeof statusIcons] ?? FolderOpen;

  return (
    <article className="project-row">
      <div className={`project-state project-state-${item.status}`} aria-hidden="true">
        <StatusIcon size={18} />
      </div>
      <div className="project-row-main">
        <div className="project-row-title">
          <h3>{item.name}</h3>
          <span className="project-status">{labels[item.status] ?? item.status}</span>
        </div>
        <p>{item.description}</p>
        <div className="project-meta">
          <span>
            <Clock3 size={14} aria-hidden="true" />{" "}
            <time dateTime={item.updatedAt}>{relativeTime(item.updatedAt)}</time>
          </span>
          <span>
            {item.progress.completed} / {item.progress.total}
          </span>
          <code>{item.workspacePath}</code>
        </div>
        {item.errorSummary && <p role="alert">{item.errorSummary}</p>}
      </div>
      <div className="project-row-actions">
        <Button type="button" onClick={onOpen}>
          開く
        </Button>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={`${item.name}のその他の操作`}
              onPointerDown={(event) => {
                trigger.current = event.currentTarget;
              }}
              onFocus={(event) => {
                trigger.current = event.currentTarget;
              }}
            >
              <MoreHorizontal size={18} />
            </Button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content className="project-menu" align="end" sideOffset={4}>
              <DropdownMenu.Item onSelect={() => onAction("duplicate", trigger.current!)}>
                <Copy size={16} aria-hidden="true" />
                複製
              </DropdownMenu.Item>
              {item.status === "error" && (
                <DropdownMenu.Item onSelect={() => onAction("reconnect", trigger.current!)}>
                  再接続
                </DropdownMenu.Item>
              )}
              {item.status === "completed" &&
                item.currentProcedureId &&
                item.currentProcedureRevision != null && (
                  <>
                    <DropdownMenu.Item onSelect={() => onAction("revision", trigger.current!)}>
                      改訂
                    </DropdownMenu.Item>
                    <DropdownMenu.Item onSelect={() => onAction("export", trigger.current!)}>
                      出力
                    </DropdownMenu.Item>
                  </>
                )}
              <DropdownMenu.Item
                disabled={item.status === "ai_running"}
                onSelect={() => onAction("archive", trigger.current!)}
              >
                <Archive size={16} aria-hidden="true" />
                アーカイブ
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="project-menu-delete"
                disabled={item.status === "ai_running"}
                onSelect={() => onAction("delete", trigger.current!)}
              >
                <Trash2 size={16} aria-hidden="true" />
                削除
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </article>
  );
}
