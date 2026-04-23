import {
  type Component,
  createResource,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { BookOpen, Library, BookMarked, Clock } from "lucide-solid";
import { api } from "../../shared/api/client";
import ScrollerRow from "./ScrollerRow";
import { t } from "../../shared/i18n/i18n";
import type { DashboardResponse } from "./types";

// Fetch dashboard data from the API.
async function fetchDashboard(): Promise<DashboardResponse> {
  return api<DashboardResponse>("/dashboard");
}

// StatCard displays a single stat with an icon, value, and label.
interface StatCardProps {
  icon: Component<{ class?: string }>;
  value: number | string;
  label: string;
}

const StatCard: Component<StatCardProps> = (props) => {
  return (
    <div class="flex items-center gap-3 rounded-lg bg-slate-800 px-4 py-3">
      <props.icon class="h-5 w-5 flex-none text-indigo-400" />
      <div class="min-w-0">
        <p class="text-lg font-bold leading-none text-slate-100">
          {props.value}
        </p>
        <p class="mt-0.5 text-xs text-slate-400">{props.label}</p>
      </div>
    </div>
  );
};

// Format seconds into a human-readable duration (e.g. "12h 30m").
function formatReadingTime(secs: number): string {
  if (secs <= 0) return "0m";
  const hours = Math.floor(secs / 3600);
  const minutes = Math.floor((secs % 3600) / 60);
  if (hours > 0 && minutes > 0) return `${hours}h ${minutes}m`;
  if (hours > 0) return `${hours}h`;
  return `${minutes}m`;
}

// Skeleton placeholder for a book card while loading.
const BookCardSkeleton: Component = () => (
  <div class="w-32 flex-none animate-pulse sm:w-36">
    <div class="aspect-[2/3] w-full rounded-md bg-slate-700" />
    <div class="mt-2 h-3 w-3/4 rounded bg-slate-700" />
    <div class="mt-1 h-2.5 w-1/2 rounded bg-slate-700" />
  </div>
);

// Skeleton for a full scroller row.
const ScrollerRowSkeleton: Component<{ title: string }> = (props) => (
  <section class="flex flex-col gap-3">
    <div class="flex items-center justify-between px-1">
      <h2 class="text-base font-semibold text-slate-100">{props.title}</h2>
    </div>
    <div class="flex gap-3 overflow-hidden pb-2">
      <For each={Array(6).fill(0)}>{() => <BookCardSkeleton />}</For>
    </div>
  </section>
);

// DashboardContent renders the loaded dashboard data.
const DashboardContent: Component<{ data: DashboardResponse }> = (props) => {
  return (
    <div class="flex flex-col gap-8 p-6">
      {/* Stats bar */}
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard
          icon={BookOpen}
          value={props.data.stats.totalBooks}
          label={t("dashboard.totalBooks")}
        />
        <StatCard
          icon={Library}
          value={props.data.stats.totalLibraries}
          label={t("dashboard.libraries")}
        />
        <StatCard
          icon={BookMarked}
          value={props.data.stats.booksReadThisMonth}
          label={t("dashboard.readThisMonth")}
        />
        <StatCard
          icon={Clock}
          value={formatReadingTime(props.data.stats.totalReadingTime)}
          label={t("dashboard.readingTime")}
        />
      </div>

      {/* Book rows */}
      <div class="flex flex-col gap-8">
        <For each={props.data.rows}>
          {(row) => (
            <ScrollerRow
              title={row.title}
              books={row.books}
            />
          )}
        </For>
      </div>
    </div>
  );
};

// DashboardLoader wraps the resource and renders loading/error states.
const DashboardLoader: Component = () => {
  const [data] = createResource(fetchDashboard);

  return (
    <Show
      when={!data.loading}
      fallback={
        <div class="flex flex-col gap-8 p-6">
          {/* Stats skeleton */}
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <For each={Array(4).fill(0)}>
              {() => (
                <div class="h-16 animate-pulse rounded-lg bg-slate-800" />
              )}
            </For>
          </div>
          {/* Row skeletons */}
          <ScrollerRowSkeleton title={t("dashboard.continueReading")} />
          <ScrollerRowSkeleton title={t("dashboard.recentlyAdded")} />
          <ScrollerRowSkeleton title={t("dashboard.randomPicks")} />
        </div>
      }
    >
      <Show when={data()} fallback={null}>
        {(d) => <DashboardContent data={d()} />}
      </Show>
    </Show>
  );
};

const Dashboard: Component = () => {
  return (
    <div class="flex flex-1 flex-col overflow-y-auto">
      <ErrorBoundary
        fallback={(err) => (
          <div class="flex flex-1 flex-col items-center justify-center gap-4 p-8">
            <BookOpen class="h-12 w-12 text-red-400/50" />
            <div class="text-center">
              <p class="text-base font-medium text-slate-200">
                {t("dashboard.failedToLoadDashboard")}
              </p>
              <p class="mt-1 text-sm text-slate-400">{err.message}</p>
            </div>
          </div>
        )}
      >
        <Suspense
          fallback={
            <div class="flex flex-col gap-8 p-6">
              <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <For each={Array(4).fill(0)}>
                  {() => (
                    <div class="h-16 animate-pulse rounded-lg bg-slate-800" />
                  )}
                </For>
              </div>
              <ScrollerRowSkeleton title={t("dashboard.continueReading")} />
              <ScrollerRowSkeleton title={t("dashboard.recentlyAdded")} />
              <ScrollerRowSkeleton title={t("dashboard.randomPicks")} />
            </div>
          }
        >
          <DashboardLoader />
        </Suspense>
      </ErrorBoundary>
    </div>
  );
};

export default Dashboard;
