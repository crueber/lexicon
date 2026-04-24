import {
  type Component,
  createResource,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useNavigate } from "@solidjs/router";
import { Library, BookOpen } from "lucide-solid";
import { api } from "../../shared/api/client";
import { t } from "../../shared/i18n/i18n";
import Skeleton from "../../shared/ui/Skeleton";

// --- Types ---

interface SeriesItem {
  id: number;
  name: string;
  bookCount: number;
}

// --- API ---

async function fetchSeries(): Promise<SeriesItem[]> {
  return api<SeriesItem[]>("/series");
}

// --- Skeleton ---

const SeriesListSkeleton: Component = () => (
  <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 p-6">
    <For each={Array.from({ length: 18 })}>
      {() => (
        <div class="flex flex-col items-center gap-3 rounded-lg bg-slate-800 p-4">
          <Skeleton class="h-16 w-16 rounded-lg" />
          <Skeleton class="h-4 w-24 rounded" />
          <Skeleton class="h-3 w-16 rounded" />
        </div>
      )}
    </For>
  </div>
);

// --- Components ---

const SeriesCard: Component<{ series: SeriesItem }> = (props) => {
  const navigate = useNavigate();
  return (
    <button
      onClick={() => navigate(`/series/${props.series.id}`)}
      class="group flex flex-col items-center gap-3 rounded-lg bg-slate-800 p-4 text-center transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900"
    >
      <div class="flex h-16 w-16 items-center justify-center rounded-lg bg-indigo-600/20 text-indigo-300 transition-colors group-hover:bg-indigo-600/30">
        <Library class="h-8 w-8" />
      </div>
      <div>
        <p class="text-sm font-medium text-slate-100">{props.series.name}</p>
        <p class="text-xs text-slate-400">
          {props.series.bookCount} {props.series.bookCount === 1 ? t("common.book") : t("common.books")}
        </p>
      </div>
    </button>
  );
};

// --- Main ---

const SeriesList: Component = () => {
  const [series] = createResource(fetchSeries);

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="border-b border-slate-800 px-6 py-5">
        <div class="flex items-center gap-3">
          <Library class="h-6 w-6 text-indigo-400" />
          <h1 class="text-xl font-bold text-slate-100">{t("common.series")}</h1>
        </div>
        <p class="mt-1 text-sm text-slate-400">{t("common.browseSeries")}</p>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">{t("common.failedToLoad")}</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<SeriesListSkeleton />}>
            <Show when={!series.loading} fallback={<SeriesListSkeleton />}>
              <Show
                when={(series() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <Library class="h-16 w-16 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">{t("common.noSeriesYet")}</p>
                  </div>
                }
              >
                <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  <For each={series()}>
                    {(s) => <SeriesCard series={s} />}
                  </For>
                </div>
              </Show>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default SeriesList;
