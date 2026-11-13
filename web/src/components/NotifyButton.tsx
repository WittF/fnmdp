import { useEffect, useState } from "react";
import { useStore } from "@nanostores/react";
import { Button, Popover } from "@heroui/react";
import {
  $subs,
  NOTIFY_CHOICES,
  permission,
  requestPermission,
  setSubs,
  toggleType,
} from "../lib/notify";
import { eventLabel } from "../lib/flag";

function BellIcon() {
  return (
    <svg
      aria-hidden
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9" />
      <path d="M13.73 21a2 2 0 0 1-3.46 0" />
    </svg>
  );
}

function ToggleRow({
  on,
  label,
  onChange,
}: {
  on: boolean;
  label: string;
  onChange: (v: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={() => onChange(!on)}
      className="flex h-9 w-full items-center justify-between gap-3 rounded-md px-2 text-left text-sm text-foreground transition-colors hover:bg-surface-hover"
    >
      <span className="min-w-0 truncate whitespace-nowrap">{label}</span>
      <span
        className={`relative h-5 w-9 shrink-0 rounded-full transition-colors ${
          on ? "bg-accent" : "bg-default-soft"
        }`}
      >
        <span
          className={`absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-background shadow-sm transition-transform ${
            on ? "translate-x-4" : "translate-x-0"
          }`}
        />
      </span>
    </button>
  );
}

export function NotifyButton() {
  const subs = useStore($subs);
  const [open, setOpen] = useState(false);
  const [denied, setDenied] = useState(false);
  const [requesting, setRequesting] = useState(false);

  useEffect(() => {
    if (!open) return;
    const isDenied = permission() === "denied";
    setDenied(isDenied);
    if (isDenied && subs.enabled) setSubs({ enabled: false });
  }, [open, subs.enabled]);

  const onEnable = async () => {
    setRequesting(true);
    setDenied(false);
    try {
      const ok = await requestPermission();
      if (ok) {
        setSubs({ enabled: true });
      } else {
        setDenied(true);
      }
    } finally {
      setRequesting(false);
    }
  };

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Button
        isIconOnly
        variant="ghost"
        size="sm"
        className={`relative h-9 min-w-9 rounded-lg p-0 transition-colors ${
          subs.enabled
            ? "bg-accent-soft text-accent-soft-foreground hover:bg-accent-soft-hover"
            : "text-muted hover:bg-default hover:text-foreground"
        }`}
        aria-label={subs.enabled ? "通知已开启" : "通知设置"}
      >
        <BellIcon />
        <span
          className={`absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full ${
            subs.enabled ? "bg-success" : "bg-muted/50"
          }`}
        />
      </Button>
      <Popover.Content className="w-80">
        <Popover.Dialog className="p-3">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <div className="text-sm font-semibold text-foreground">通知设置</div>
              <div className="text-xs text-muted">仅标签页打开时触发</div>
            </div>
            <span
              className={`rounded-md px-2 py-1 text-xs ${
                subs.enabled ? "bg-success-soft text-success-soft-foreground" : "bg-default-soft text-muted"
              }`}
            >
              {subs.enabled ? "已开启" : "未开启"}
            </span>
          </div>
          {denied && (
            <p className="mb-2 rounded-md bg-danger-soft px-2 py-1.5 text-xs text-danger-soft-foreground">
              浏览器已拒绝通知权限，请在站点设置中允许。
            </p>
          )}
          {!subs.enabled ? (
            <div className="space-y-3">
              {denied ? null : (
                <p className="rounded-md bg-default-soft px-2 py-2 text-xs leading-5 text-muted">
                  开启后，命中默认提醒事件时会发送浏览器通知。
                </p>
              )}
              {denied ? null : (
                <Button fullWidth size="sm" variant="primary" onPress={onEnable} isDisabled={requesting}>
                  {requesting ? "请求中" : "开启通知"}
                </Button>
              )}
            </div>
          ) : (
            <>
              <div className="mb-1 px-2 text-xs font-medium text-muted">提醒事件</div>
              <div className="space-y-1">
                {NOTIFY_CHOICES.map((t) => (
                  <ToggleRow key={t} on={!!subs.types[t]} label={eventLabel[t] ?? t} onChange={() => toggleType(t)} />
                ))}
              </div>
              <div className="my-2 h-px bg-separator" />
              <ToggleRow on={subs.sound} label="提示音" onChange={(v) => setSubs({ sound: v })} />
              <Button
                variant="danger-soft"
                size="sm"
                fullWidth
                className="mt-3"
                onPress={() => {
                  setSubs({ enabled: false });
                  setOpen(false);
                }}
              >
                关闭通知
              </Button>
            </>
          )}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}
