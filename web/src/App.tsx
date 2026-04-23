import { type Component, type JSX, Show, createSignal, createMemo } from "solid-js";
import { Router, Route, A, useLocation } from "@solidjs/router";
import {
  BookOpen,
  LayoutDashboard,
  Library,
  BookMarked,
  Settings,
  Users,
  LogOut,
  Menu,
  X,
  Notebook as NotebookIcon,
  ClipboardList,
} from "lucide-solid";
import { t } from "./shared/i18n/i18n";
import AuthProvider, { useAuth } from "./features/auth/AuthProvider";
import LoginPage from "./features/auth/LoginPage";
import ProtectedRoute from "./features/auth/ProtectedRoute";
import SettingsPage from "./features/auth/SettingsPage";
import Dashboard from "./features/dashboard/Dashboard";
import LibraryList from "./features/library/LibraryList";
import LibraryBrowser from "./features/library/LibraryBrowser";
import BookDetail from "./features/book/BookDetail";
import EpubReader from "./features/reader/EpubReader";
import PdfReader from "./features/reader/PdfReader";
import ComicReader from "./features/reader/ComicReader";
import AudiobookPlayer from "./features/reader/AudiobookPlayer";
import ReaderDispatch from "./features/reader/ReaderDispatch";
import ShelfList from "./features/shelf/ShelfList";
import ShelfDetail from "./features/shelf/ShelfDetail";
import MagicShelfBuilder from "./features/shelf/MagicShelfBuilder";
import MagicShelfDetail from "./features/shelf/MagicShelfDetail";
import UserManagement from "./features/admin/UserManagement";
import AuditLogs from "./features/admin/AuditLogs";
import Notebook from "./features/notebook/Notebook";
import ToastContainer from "./shared/ui/Toast";
import WSProvider from "./shared/ws/WSProvider";

const AdminSettingsStub: Component = () => (
  <div class="flex flex-1 items-center justify-center p-8">
    <p class="text-slate-400">{t("common.adminSettingsComingSoon")}</p>
  </div>
);

// --- Sidebar ---

interface NavItemProps {
  href: string;
  icon: Component<{ class?: string }>;
  label: string;
  onClick?: () => void;
}

const NavItem: Component<NavItemProps> = (props) => {
  const location = useLocation();
  const isActive = createMemo(() => {
    const path = location.pathname;
    return path === props.href || (props.href !== "/" && path.startsWith(props.href));
  });

  return (
    <A
      href={props.href}
      onClick={props.onClick}
      class={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
        isActive()
          ? "bg-indigo-600/20 text-indigo-300"
          : "text-slate-400 hover:bg-slate-800 hover:text-slate-200"
      }`}
    >
      <props.icon class="h-5 w-5 shrink-0" />
      <span>{props.label}</span>
    </A>
  );
};

const Sidebar: Component = () => {
  const auth = useAuth();

  return (
    <aside class="hidden md:flex md:w-64 md:flex-col md:border-r md:border-slate-800 bg-slate-900">
      {/* Logo */}
      <div class="flex items-center gap-3 px-5 py-5 border-b border-slate-800">
        <BookOpen class="h-7 w-7 text-indigo-400" />
        <span class="text-lg font-bold text-slate-100">Lexicon</span>
      </div>

      {/* Navigation */}
      <nav class="flex flex-1 flex-col gap-1 px-3 py-4">
        <NavItem href="/" icon={LayoutDashboard} label={t("common.dashboard")} />
        <NavItem href="/libraries" icon={Library} label={t("common.libraries")} />
        <NavItem href="/shelves" icon={BookMarked} label={t("common.shelves")} />
        <NavItem href="/notebook" icon={NotebookIcon} label={t("common.notebook")} />

        <Show when={auth.isAdmin()}>
          <div class="mt-6 mb-2 px-3">
            <span class="text-xs font-semibold uppercase tracking-wider text-slate-500">
              {t("common.admin")}
            </span>
          </div>
          <NavItem href="/admin/users" icon={Users} label={t("common.users")} />
          <NavItem href="/admin/audit-logs" icon={ClipboardList} label={t("common.auditLogs")} />
          <NavItem href="/admin/settings" icon={Settings} label={t("common.settings")} />
        </Show>
      </nav>

      {/* User menu */}
      <div class="border-t border-slate-800 px-3 py-4">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3 min-w-0">
            <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-indigo-600/30 text-sm font-medium text-indigo-300">
              {auth.user()?.username?.charAt(0).toUpperCase() ?? "?"}
            </div>
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-slate-200">
                {auth.user()?.username ?? ""}
              </p>
              <p class="truncate text-xs text-slate-500">
                {auth.user()?.role ?? ""}
              </p>
            </div>
          </div>
          <button
            onClick={() => auth.logout()}
            class="rounded-lg p-2 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
            title={t("common.signOut")}
          >
            <LogOut class="h-4 w-4" />
          </button>
        </div>
      </div>
    </aside>
  );
};

// --- Mobile Bottom Nav ---

const MobileNav: Component = () => {
  const auth = useAuth();
  const [menuOpen, setMenuOpen] = createSignal(false);

  return (
    <>
      {/* Bottom navigation bar */}
      <nav class="fixed bottom-0 left-0 right-0 z-40 flex items-center justify-around border-t border-slate-800 bg-slate-900 px-2 py-2 md:hidden">
        <MobileNavItem href="/" icon={LayoutDashboard} label={t("common.home")} />
        <MobileNavItem href="/libraries" icon={Library} label={t("common.libraries")} />
        <MobileNavItem href="/shelves" icon={BookMarked} label={t("common.shelves")} />
        <MobileNavItem href="/notebook" icon={NotebookIcon} label={t("common.notebook")} />
        <button
          onClick={() => setMenuOpen((v) => !v)}
          class="flex flex-col items-center gap-0.5 rounded-lg px-3 py-1.5 text-slate-400 hover:text-slate-200 transition-colors"
        >
          <Show when={menuOpen()} fallback={<Menu class="h-5 w-5" />}>
            <X class="h-5 w-5" />
          </Show>
          <span class="text-[10px]">{t("common.more")}</span>
        </button>
      </nav>

      {/* Mobile menu overlay */}
      <Show when={menuOpen()}>
        <div
          class="fixed inset-0 z-30 bg-black/50 md:hidden"
          onClick={() => setMenuOpen(false)}
        />
        <div class="fixed bottom-14 left-0 right-0 z-30 rounded-t-xl border-t border-slate-800 bg-slate-900 p-4 md:hidden">
          <div class="flex flex-col gap-2">
            <Show when={auth.isAdmin()}>
              <p class="px-3 text-xs font-semibold uppercase tracking-wider text-slate-500">
                {t("common.admin")}
              </p>
              <NavItem href="/admin/users" icon={Users} label={t("common.users")} onClick={() => setMenuOpen(false)} />
              <NavItem href="/admin/audit-logs" icon={ClipboardList} label={t("common.auditLogs")} onClick={() => setMenuOpen(false)} />
              <NavItem href="/admin/settings" icon={Settings} label={t("common.settings")} onClick={() => setMenuOpen(false)} />
              <div class="my-2 border-t border-slate-800" />
            </Show>
            <NavItem href="/settings" icon={Settings} label={t("common.settings")} onClick={() => setMenuOpen(false)} />
            <button
              onClick={() => {
                setMenuOpen(false);
                auth.logout();
              }}
              class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-red-400 hover:bg-slate-800 transition-colors"
            >
              <LogOut class="h-5 w-5 shrink-0" />
              <span>{t("common.signOut")}</span>
            </button>
          </div>
        </div>
      </Show>
    </>
  );
};

const MobileNavItem: Component<{ href: string; icon: Component<{ class?: string }>; label: string }> = (props) => {
  const location = useLocation();
  const isActive = createMemo(() => {
    const path = location.pathname;
    return path === props.href || (props.href !== "/" && path.startsWith(props.href));
  });

  return (
    <A
      href={props.href}
      class={`flex flex-col items-center gap-0.5 rounded-lg px-3 py-1.5 transition-colors ${
        isActive()
          ? "text-indigo-400"
          : "text-slate-400 hover:text-slate-200"
      }`}
    >
      <props.icon class="h-5 w-5" />
      <span class="text-[10px]">{props.label}</span>
    </A>
  );
};

// --- App Layout ---

const AppLayout: Component<{ children?: any }> = (props) => {
  return (
    <ProtectedRoute>
      <div class="flex min-h-screen bg-slate-900 text-slate-100">
        <Sidebar />
        <main class="flex flex-1 flex-col pb-16 md:pb-0">
          {props.children}
        </main>
        <MobileNav />
      </div>
    </ProtectedRoute>
  );
};

// AuthenticatedProviders wraps children with providers that depend on auth state.
// Must be rendered inside AuthProvider.
const AuthenticatedProviders: Component<{ children: JSX.Element }> = (props) => {
  const auth = useAuth();
  return (
    <WSProvider isAuthenticated={auth.isAuthenticated}>
      {props.children}
    </WSProvider>
  );
};

// --- App ---

const App: Component = () => {
  return (
    <Router
      root={(props) => (
        <AuthProvider>
          <AuthenticatedProviders>
            {props.children}
            <ToastContainer />
          </AuthenticatedProviders>
        </AuthProvider>
      )}
    >
      <Route path="/login" component={LoginPage} />
      {/* Reader routes: full-screen, no sidebar, but still require auth */}
      <Route path="/books/:id/read" component={ReaderDispatch} />
      <Route path="/books/:id/read/epub" component={EpubReader} />
      <Route path="/books/:id/read/pdf" component={PdfReader} />
      <Route path="/books/:id/read/comic" component={ComicReader} />
      <Route path="/books/:id/read/audio" component={AudiobookPlayer} />
      <Route path="/" component={AppLayout}>
        <Route path="/" component={Dashboard} />
        <Route path="/libraries" component={LibraryList} />
        <Route path="/libraries/:id/books" component={LibraryBrowser} />
        <Route path="/books/:id" component={BookDetail} />
        <Route path="/shelves" component={ShelfList} />
        <Route path="/shelves/:id" component={ShelfDetail} />
        <Route path="/magic-shelves/new" component={MagicShelfBuilder} />
        <Route path="/magic-shelves/:id" component={MagicShelfDetail} />
        <Route path="/magic-shelves/:id/edit" component={MagicShelfBuilder} />
        <Route path="/notebook" component={Notebook} />
        <Route path="/settings" component={SettingsPage} />
        <Route path="/admin/users" component={UserManagement} />
        <Route path="/admin/audit-logs" component={AuditLogs} />
        <Route path="/admin/settings" component={AdminSettingsStub} />
      </Route>
    </Router>
  );
};

export default App;
