// The project's SKU board: a single-column, drag-reorderable list of cards with
// optional aggregation groups. One global time control drives every ungrouped card;
// each group can override it for its members ("聚合统一控制"). Drag (motion Reorder)
// gives the dynamic feel; grouping is via each card's ⋯ menu.
import { useEffect, useMemo } from "react";
import { useStore } from "@nanostores/react";
import { Reorder, useDragControls } from "motion/react";
import type { DragControls } from "motion/react";
import { Button, Card, Dropdown, Header, Label, Separator, Tooltip } from "@heroui/react";
import { $screens, $skus, projectScreens, projectSkus, screenKey } from "../store";
import {
  $global,
  $groupWin,
  clearGroupWin,
  panWin,
  pickWin,
  setGlobalWin,
  setGroupWin,
} from "../lib/statusWindow";
import { useProjectTrajectories, type TrajectoryMap } from "../lib/trajectories";
import {
  $focusedSku,
  $layouts,
  addToGroup,
  autoGroupByScreen,
  clearFocus,
  createGroup,
  dissolveAll,
  dissolveGroup,
  ensureDefaultLayout,
  removeFromGroup,
  renameGroup,
  resolveLayout,
  setGroupMembersOrder,
  setTopOrderByIds,
  type ResolvedBlock,
} from "../lib/skuLayout";
import { StatusLegend, StatusWindowControl } from "./StatusWindowControl";
import { SkuCard } from "./SkuCard";
import { FULL_REFRESH_MS } from "./StatusGrid";

type GroupRef = { id: string; name: string };

const topId = (b: ResolvedBlock) => (b.kind === "sku" ? `sku:${b.key}` : `grp:${b.id}`);

function GripIcon() {
  return (
    <svg aria-hidden width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
      <circle cx="9" cy="6" r="1.5" />
      <circle cx="15" cy="6" r="1.5" />
      <circle cx="9" cy="12" r="1.5" />
      <circle cx="15" cy="12" r="1.5" />
      <circle cx="9" cy="18" r="1.5" />
      <circle cx="15" cy="18" r="1.5" />
    </svg>
  );
}

function EllipsisIcon() {
  return (
    <svg aria-hidden width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
      <circle cx="5" cy="12" r="1.6" />
      <circle cx="12" cy="12" r="1.6" />
      <circle cx="19" cy="12" r="1.6" />
    </svg>
  );
}

function Handle({ controls }: { controls: DragControls }) {
  return (
    <Tooltip delay={500}>
      <Tooltip.Trigger>
        <button
          type="button"
          onPointerDown={(e) => controls.start(e)}
          className="cursor-grab touch-none px-1 text-muted hover:text-foreground"
          aria-label="拖拽排序"
        >
          <GripIcon />
        </button>
      </Tooltip.Trigger>
      <Tooltip.Content>
        <p>拖拽排序</p>
      </Tooltip.Content>
    </Tooltip>
  );
}

function CardMenu({
  pid,
  cardKey,
  groups,
  inGroupId,
  defaultName,
}: {
  pid: number;
  cardKey: string;
  groups: GroupRef[];
  inGroupId?: string;
  defaultName: string;
}) {
  return (
    <Dropdown>
      <Dropdown.Trigger>
        <Button isIconOnly size="sm" variant="ghost" className="h-7 min-w-7 p-0 text-muted" aria-label="更多操作">
          <EllipsisIcon />
        </Button>
      </Dropdown.Trigger>
      <Dropdown.Popover className="min-w-44">
        <Dropdown.Menu
          onAction={(key) => {
            const k = String(key);
            if (k === "new") createGroup(pid, [cardKey], defaultName);
            else if (k === "leave" && inGroupId) removeFromGroup(pid, inGroupId, cardKey);
            else if (k.startsWith("to:")) addToGroup(pid, k.slice(3), cardKey);
          }}
        >
          {inGroupId ? (
            <Dropdown.Item id="leave" textValue="移出分组">
              <Label>移出分组</Label>
            </Dropdown.Item>
          ) : (
            <>
              <Dropdown.Item id="new" textValue="新建分组">
                <Label>新建分组</Label>
              </Dropdown.Item>
              {groups.length > 0 && (
                <Dropdown.Section>
                  <Header>移入分组</Header>
                  {groups.map((g) => (
                    <Dropdown.Item key={g.id} id={`to:${g.id}`} textValue={g.name}>
                      <Label>{g.name}</Label>
                    </Dropdown.Item>
                  ))}
                </Dropdown.Section>
              )}
            </>
          )}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}

function GroupMenu({ pid, id, name }: { pid: number; id: string; name: string }) {
  return (
    <Dropdown>
      <Dropdown.Trigger>
        <Button isIconOnly size="sm" variant="ghost" className="h-7 min-w-7 p-0 text-muted" aria-label="分组操作">
          <EllipsisIcon />
        </Button>
      </Dropdown.Trigger>
      <Dropdown.Popover className="min-w-36">
        <Dropdown.Menu
          onAction={(key) => {
            const k = String(key);
            if (k === "rename") {
              const n = window.prompt("分组名称", name);
              if (n && n.trim()) renameGroup(pid, id, n.trim());
            } else if (k === "dissolve") {
              dissolveGroup(pid, id);
            }
          }}
        >
          <Dropdown.Item id="rename" textValue="重命名">
            <Label>重命名</Label>
          </Dropdown.Item>
          <Dropdown.Item id="dissolve" textValue="解散分组">
            <Label>解散分组</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}

function MemberItem({
  member,
  pid,
  groupId,
  groups,
  win,
  focusedKey,
  trajectories,
}: {
  member: { key: string; sku: import("../api/rest").RestSku };
  pid: number;
  groupId: string;
  groups: GroupRef[];
  win: import("../lib/statusWindow").Win;
  focusedKey: string | null;
  trajectories: TrajectoryMap | null;
}) {
  const controls = useDragControls();
  const screensMap = useStore($screens);
  return (
    <Reorder.Item as="div" id={`skucard-${member.key}`} value={member.key} dragListener={false} dragControls={controls}>
      <SkuCard
        sku={member.sku}
        screen={screensMap[screenKey(pid, member.sku.screen_id)]}
        win={win}
        highlight={focusedKey === member.key}
        trajectory={trajectories?.[member.key] ?? null}
        leading={<Handle controls={controls} />}
        actions={
          <CardMenu pid={pid} cardKey={member.key} groups={groups} inGroupId={groupId} defaultName={member.sku.desc || "分组"} />
        }
      />
    </Reorder.Item>
  );
}

function GroupBlock({
  block,
  pid,
  groups,
  handle,
  focusedKey,
  globalTrajectories,
}: {
  block: Extract<ResolvedBlock, { kind: "group" }>;
  pid: number;
  groups: GroupRef[];
  handle: React.ReactNode;
  focusedKey: string | null;
  globalTrajectories: TrajectoryMap | null;
}) {
  const global = useStore($global);
  const groupWin = useStore($groupWin);
  const win = pickWin(global, groupWin, block.id);
  const override = block.id in groupWin;
  const groupTrajectories = useProjectTrajectories(pid, win, FULL_REFRESH_MS, override);
  const trajectories = override ? groupTrajectories : globalTrajectories;
  const memberKeys = block.members.map((m) => m.key);
  // Other groups available as move targets (exclude self).
  const others = groups.filter((g) => g.id !== block.id);

  return (
    <div className="space-y-2 rounded-lg border border-separator/60 bg-surface-secondary p-2">
      <div className="flex items-center gap-2">
        {handle}
        <span className="text-sm font-medium text-foreground">{block.name}</span>
        <span className="text-xs text-muted">({block.members.length})</span>
        <span className="ml-auto">
          <GroupMenu pid={pid} id={block.id} name={block.name} />
        </span>
      </div>
      <StatusWindowControl
        win={win}
        onWindow={(ms) => setGroupWin(block.id, { intervalMs: ms })}
        onPan={(dir) => setGroupWin(block.id, { anchorMs: panWin(win, dir).anchorMs })}
        onReset={() => setGroupWin(block.id, { anchorMs: null })}
        trailing={
          override && (
            <Button size="sm" variant="ghost" onPress={() => clearGroupWin(block.id)}>
              跟随全局
            </Button>
          )
        }
      />
      <Reorder.Group
        as="div"
        axis="y"
        values={memberKeys}
        onReorder={(keys: string[]) => setGroupMembersOrder(pid, block.id, keys)}
        className="space-y-2"
      >
        {block.members.map((m) => (
          <MemberItem
            key={m.key}
            member={m}
            pid={pid}
            groupId={block.id}
            groups={others}
            win={win}
            focusedKey={focusedKey}
            trajectories={trajectories}
          />
        ))}
      </Reorder.Group>
    </div>
  );
}

function TopBlock({
  block,
  pid,
  groups,
  focusedKey,
  globalTrajectories,
}: {
  block: ResolvedBlock;
  pid: number;
  groups: GroupRef[];
  focusedKey: string | null;
  globalTrajectories: TrajectoryMap | null;
}) {
  const controls = useDragControls();
  const global = useStore($global);
  const screensMap = useStore($screens);
  return (
    <Reorder.Item
      as="div"
      id={block.kind === "sku" ? `skucard-${block.key}` : undefined}
      value={topId(block)}
      dragListener={false}
      dragControls={controls}
    >
      {block.kind === "sku" ? (
        <SkuCard
          sku={block.sku}
          screen={screensMap[screenKey(pid, block.sku.screen_id)]}
          win={global}
          highlight={focusedKey === block.key}
          trajectory={globalTrajectories?.[block.key] ?? null}
          leading={<Handle controls={controls} />}
          actions={
            <CardMenu pid={pid} cardKey={block.key} groups={groups} defaultName={block.sku.desc || "分组"} />
          }
        />
      ) : (
        <GroupBlock
          block={block}
          pid={pid}
          groups={groups}
          handle={<Handle controls={controls} />}
          focusedKey={focusedKey}
          globalTrajectories={globalTrajectories}
        />
      )}
    </Reorder.Item>
  );
}

export function SkuBoard({ pid }: { pid: number }) {
  const skusMap = useStore($skus);
  const screensMap = useStore($screens);
  const layouts = useStore($layouts);
  const global = useStore($global);
  const focusedKey = useStore($focusedSku);

  // Hydrate the saved layout and, for a never-customized project, default to
  // aggregating by screen. Re-runs as skus/screens stream in so it groups once
  // both are present.
  useEffect(() => {
    ensureDefaultLayout(pid, projectScreens(pid, screensMap), projectSkus(pid, skusMap));
  }, [pid, skusMap, screensMap]);

  // After a jump-from-event, scroll the targeted card into view and let its
  // highlight ring fade out.
  useEffect(() => {
    if (!focusedKey) return;
    document.getElementById(`skucard-${focusedKey}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
    const t = setTimeout(() => clearFocus(), 2500);
    return () => clearTimeout(t);
  }, [focusedKey]);

  const skus = useMemo(() => projectSkus(pid, skusMap), [pid, skusMap]);
  const blocks = useMemo(() => resolveLayout(layouts[String(pid)], skus), [layouts, pid, skus]);
  const globalTrajectories = useProjectTrajectories(pid, global, FULL_REFRESH_MS, skus.length > 0);
  const groups: GroupRef[] = useMemo(
    () => blocks.filter((b) => b.kind === "group").map((b) => ({ id: b.id, name: b.name })),
    [blocks],
  );

  if (skus.length === 0) {
    return <Card className="px-4 py-12 text-center text-sm text-muted">等待节点回填场次/票档…</Card>;
  }

  const topIds = blocks.map(topId);

  return (
    <Card className="gap-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-semibold text-foreground">票档状态</span>
        <div className="flex items-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            onPress={() => autoGroupByScreen(pid, projectScreens(pid, screensMap), skus)}
          >
            按场次聚合
          </Button>
          <Button size="sm" variant="ghost" onPress={() => dissolveAll(pid)}>
            全部解散
          </Button>
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <StatusWindowControl
          win={global}
          onWindow={(ms) => setGlobalWin({ intervalMs: ms })}
          onPan={(dir) => setGlobalWin({ anchorMs: panWin(global, dir).anchorMs })}
          onReset={() => setGlobalWin({ anchorMs: null })}
        />
        <StatusLegend />
      </div>
      <Separator />
      <Reorder.Group
        as="div"
        axis="y"
        values={topIds}
        onReorder={(ids: string[]) => setTopOrderByIds(pid, ids)}
        className="space-y-3"
      >
        {blocks.map((b) => (
          <TopBlock
            key={topId(b)}
            block={b}
            pid={pid}
            groups={groups}
            focusedKey={focusedKey}
            globalTrajectories={globalTrajectories}
          />
        ))}
      </Reorder.Group>
    </Card>
  );
}
