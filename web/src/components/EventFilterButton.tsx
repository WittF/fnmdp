import { useStore } from "@nanostores/react";
import { Button, Dropdown, Header, Label } from "@heroui/react";
import type { Key, Selection } from "@heroui/react";
import { $eventFilter, EVENT_GROUPS, setHidden, showAll } from "../lib/eventFilter";
import type { EventType } from "../protocol/types";
import { eventLabel } from "../lib/flag";

const ALL_TYPES: EventType[] = EVENT_GROUPS.flatMap((g) => g.types);

function FilterIcon() {
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
      <path d="M22 3H2l8 9.46V19l4 2v-8.54L22 3z" />
    </svg>
  );
}

export function EventFilterButton() {
  const filter = useStore($eventFilter);
  const active = filter.hidden.length > 0;
  // Menu tracks the VISIBLE (selected) keys; store tracks HIDDEN — convert at the edge.
  const visible = new Set<Key>(ALL_TYPES.filter((t) => !filter.hidden.includes(t)));

  const onSelectionChange = (keys: Selection) => {
    if (keys === "all") {
      showAll();
      return;
    }
    setHidden(ALL_TYPES.filter((t) => !keys.has(t)));
  };

  return (
    <Dropdown>
      <Dropdown.Trigger>
        <Button
          isIconOnly
          variant="ghost"
          size="sm"
          className={`relative h-8 min-w-8 rounded-lg p-0 transition-colors ${
            active
              ? "bg-accent-soft text-accent-soft-foreground hover:bg-accent-soft-hover"
              : "text-muted hover:bg-default hover:text-foreground"
          }`}
          aria-label="筛选事件类型"
        >
          <FilterIcon />
          {active && (
            <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-accent" />
          )}
        </Button>
      </Dropdown.Trigger>
      <Dropdown.Popover className="min-w-56">
        <Dropdown.Menu
          selectionMode="multiple"
          selectedKeys={visible}
          onSelectionChange={onSelectionChange}
          aria-label="显示哪些事件"
        >
          {EVENT_GROUPS.map((group) => (
            <Dropdown.Section key={group.key}>
              <Header>{group.label}</Header>
              {group.types.map((t) => (
                <Dropdown.Item key={t} id={t} textValue={eventLabel[t] ?? t}>
                  <Dropdown.ItemIndicator />
                  <Label>{eventLabel[t] ?? t}</Label>
                </Dropdown.Item>
              ))}
            </Dropdown.Section>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
