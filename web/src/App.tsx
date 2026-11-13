import { useEffect } from "react";
import type { Key } from "react-aria-components";
import { BrowserRouter, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { Navbar, Segment } from "@heroui-pro/react";
import { bootstrap } from "./store/connect";
import { ConnBadge } from "./components/primitives";
import { NotifyButton } from "./components/NotifyButton";
import { ThemeToggle } from "./components/ThemeToggle";
import { Overview } from "./pages/Overview";
import { ProjectDetail } from "./pages/ProjectDetail";
import { Events } from "./pages/Events";
import { Health } from "./pages/Health";

const NAV = [
  { key: "overview", to: "/", label: "状态墙" },
  { key: "events", to: "/events", label: "事件流" },
  { key: "health", to: "/health", label: "监控健康" },
] as const;

function BrandLogo() {
  return (
    <img src="/logo.png" alt="" className="size-7 shrink-0 rounded-lg" />
  );
}

function TopBar() {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const activeNavKey = pathname.startsWith("/events")
    ? "events"
    : pathname.startsWith("/health")
      ? "health"
      : "overview";

  const onNavChange = (key: Key) => {
    const target = NAV.find((n) => n.key === key);
    if (target) navigate(target.to);
  };

  return (
    <div className="px-4 py-3">
      <div className="mx-auto max-w-[1500px] rounded-xl border border-border bg-background md:overflow-hidden">
        <Navbar height="3.25rem" maxWidth="full" navigate={navigate} position="static" size="sm">
          <Navbar.Header>
            <Navbar.Brand>
              <BrandLogo />
              <span className="text-sm font-semibold text-foreground">fnmdp</span>
              <span className="hidden text-xs font-normal text-muted sm:inline">票务监控</span>
            </Navbar.Brand>

            <Navbar.Spacer />

            <Navbar.Content className="hidden md:flex">
              <Segment
                aria-label="主导航"
                selectedKey={activeNavKey}
                size="sm"
                variant="ghost"
                onSelectionChange={onNavChange}
              >
                {NAV.map((n, i) => (
                  <Segment.Item key={n.key} id={n.key}>
                    {i > 0 && <Segment.Separator />}
                    {n.label}
                  </Segment.Item>
                ))}
              </Segment>
            </Navbar.Content>

            <Navbar.Spacer />

            <Navbar.Content>
              <span className="hidden lg:inline-flex">
                <ConnBadge />
              </span>
              <Navbar.Separator className="hidden lg:block" />
              <NotifyButton />
              <ThemeToggle />
              <Navbar.MenuToggle className="md:hidden" srLabel="切换导航菜单" />
            </Navbar.Content>
          </Navbar.Header>
          <Navbar.Menu className="rounded-b-xl border-t border-separator">
            {NAV.map((n) => (
              <Navbar.MenuItem key={n.key} href={n.to} isCurrent={activeNavKey === n.key}>
                {n.label}
              </Navbar.MenuItem>
            ))}
          </Navbar.Menu>
        </Navbar>
      </div>
    </div>
  );
}

export function App() {
  useEffect(() => {
    const h = bootstrap();
    return () => h.stop();
  }, []);

  return (
    <BrowserRouter>
      <div className="min-h-screen bg-background text-foreground">
        <TopBar />
        <main className="mx-auto max-w-[1500px] p-4">
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/project/:id" element={<ProjectDetail />} />
            <Route path="/events" element={<Events />} />
            <Route path="/health" element={<Health />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}
