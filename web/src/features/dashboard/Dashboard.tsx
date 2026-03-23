import { type Component, Show } from "solid-js";
import { BookOpen } from "lucide-solid";
import { useAuth } from "../auth/AuthProvider";

const Dashboard: Component = () => {
  const auth = useAuth();

  return (
    <div class="flex flex-1 flex-col items-center justify-center gap-6 p-8">
      <BookOpen class="h-16 w-16 text-indigo-400/50" />
      <div class="text-center">
        <h1 class="text-2xl font-bold text-slate-100">
          Welcome to Lexicon
          <Show when={auth.user()?.name}>
            {(name) => <>, {name()}</>}
          </Show>
        </h1>
        <p class="mt-2 text-slate-400">
          Your self-hosted digital library
        </p>
      </div>
    </div>
  );
};

export default Dashboard;
