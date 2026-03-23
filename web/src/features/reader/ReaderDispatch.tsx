import { type Component, createResource, Show } from "solid-js";
import { useParams, useNavigate, Navigate } from "@solidjs/router";
import { Loader2 } from "lucide-solid";
import { api } from "../../shared/api/client";
import type { BookFile } from "../library/types";

// Fetch the list of files for a book.
async function fetchBookFiles(bookId: string): Promise<BookFile[]> {
  return api<BookFile[]>(`/books/${bookId}/files`);
}

// Determine the best file to read from the list.
// Preference order: EPUB > PDF > CBZ > CBR > CB7 > anything else.
function pickBestFile(files: BookFile[]): BookFile | undefined {
  const epub = files.find((f) => f.format === "EPUB");
  if (epub) return epub;
  const pdf = files.find((f) => f.format === "PDF");
  if (pdf) return pdf;
  const cbz = files.find((f) => f.format === "CBZ");
  if (cbz) return cbz;
  const cbr = files.find((f) => f.format === "CBR");
  if (cbr) return cbr;
  const cb7 = files.find((f) => f.format === "CB7");
  if (cb7) return cb7;
  return files[0];
}

// Map a file format to the reader route segment.
function readerRouteForFormat(format: string): string {
  switch (format) {
    case "PDF":
      return "pdf";
    case "CBZ":
    case "CBR":
    case "CB7":
      return "comic";
    default:
      return "epub";
  }
}

// ReaderDispatch fetches the book's files and redirects to the appropriate reader.
const ReaderDispatch: Component = () => {
  const params = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [files] = createResource(() => params.id, fetchBookFiles);

  // Compute the redirect target once files are loaded.
  const redirectTarget = () => {
    const f = files();
    if (!f) return null;
    const best = pickBestFile(f);
    if (!best) return null;
    const readerType = readerRouteForFormat(best.format);
    return `/books/${params.id}/read/${readerType}?fileId=${best.id}`;
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-slate-950">
      <Show
        when={!files.loading && !files.error}
        fallback={
          <Show
            when={files.error}
            fallback={
              <div class="flex flex-col items-center gap-3 text-slate-400">
                <Loader2 class="h-8 w-8 animate-spin text-indigo-400" />
                <p class="text-sm">Loading book…</p>
              </div>
            }
          >
            <div class="flex flex-col items-center gap-3 text-center">
              <p class="text-lg font-medium text-red-400">Failed to load book</p>
              <button
                onClick={() => navigate(-1)}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                ← Go back
              </button>
            </div>
          </Show>
        }
      >
        <Show
          when={redirectTarget()}
          fallback={
            <div class="flex flex-col items-center gap-3 text-center">
              <p class="text-lg font-medium text-slate-300">No readable files found</p>
              <button
                onClick={() => navigate(-1)}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                ← Go back
              </button>
            </div>
          }
        >
          {(target) => <Navigate href={target()} />}
        </Show>
      </Show>
    </div>
  );
};

export default ReaderDispatch;
