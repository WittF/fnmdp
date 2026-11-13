import { Button, Dropdown, Description, Label } from "@heroui/react";

// 会员购 has two detail surfaces for the same project id: the desktop 演出 page
// (show.bilibili.com) and the mobile 商城 page (mall.bilibili.com). We offer both
// rather than guess from the user agent, since people often open on the "wrong" device.
const PC_URL = (id: number) => `https://show.bilibili.com/platform/detail.html?id=${id}`;
const MOBILE_URL = (id: number) => `https://mall.bilibili.com/neul-next/ticket-renovation/detail.html?id=${id}`;

function ExternalLinkIcon() {
  return (
    <svg
      aria-hidden
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M15 3h6v6" />
      <path d="M10 14 21 3" />
      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
    </svg>
  );
}

export function MallLinkButton({ pid }: { pid: number }) {
  return (
    <Dropdown>
      <Dropdown.Trigger>
        <Button variant="secondary" size="sm" className="shrink-0 gap-1.5">
          <ExternalLinkIcon />
          会员购详情
        </Button>
      </Dropdown.Trigger>
      <Dropdown.Popover className="min-w-48">
        <Dropdown.Menu
          aria-label="前往会员购详情"
          onAction={(key) => {
            const url = key === "mobile" ? MOBILE_URL(pid) : PC_URL(pid);
            window.open(url, "_blank", "noopener,noreferrer");
          }}
        >
          <Dropdown.Item id="pc" textValue="电脑端 网页">
            <Label>电脑端</Label>
            <Description>show.bilibili.com 演出页</Description>
          </Dropdown.Item>
          <Dropdown.Item id="mobile" textValue="手机端 商城">
            <Label>手机端</Label>
            <Description>mall.bilibili.com 商城页</Description>
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
