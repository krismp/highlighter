import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, type AddError, type CombineChoices, type CombineJob, type CombineView } from "@/lib/api";
import { reportError } from "@/lib/errors";

const key = ["combine"];

/** Shows the files that could not be added, in one alert. */
export function reportAddErrors(errors: AddError[]) {
  if (errors.length === 0) return;
  const lines = errors.map((e) => `${e.name}: ${e.error}`).join("\n");
  reportError(errors.length === 1 ? `Couldn't add ${errors[0].name}` : `Couldn't add ${errors.length} files`,
    errors.length === 1 ? errors[0].error : lines, errors.map((e) => e.path).join("\n"));
}

/** Adds videos by path to the Combine line-up, from anywhere in the app. */
export function useAddVideos() {
  const qc = useQueryClient();
  return useMutation({
    meta: { errorTitle: "Couldn't add the videos" },
    mutationFn: api.addVideos,
    onSuccess: ({ combine, errors }) => {
      qc.setQueryData(key, combine);
      reportAddErrors(errors);
    },
  });
}

/** The Combine line-up and choices, kept on the server, with live progress of a running combine. */
export function useCombine() {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: key, queryFn: api.combine, meta: { errorTitle: "Couldn't load the videos to combine" } });

  useEffect(() => {
    const es = new EventSource(api.combineEventsUrl);
    es.onmessage = (e) => {
      try {
        const job = JSON.parse(e.data) as CombineJob;
        const before = qc.getQueryData<CombineView>(key)?.job.status;
        qc.setQueryData<CombineView>(key, (v) => (v ? { ...v, job } : v));
        if (job.status === "done" && before !== "done") qc.invalidateQueries({ queryKey: ["library"] }); // the new file is in the folder
      } catch (err) {
        reportError("Couldn't read combine progress", err, `event data: ${String(e.data).slice(0, 300)}`);
      }
    };
    return () => es.close();
  }, [qc]);

  // Optimistic: reordering and choices show at once, the server confirms.
  const save = useMutation({
    meta: { errorTitle: "Couldn't save the changes" },
    mutationFn: ({ itemIds, choices }: { itemIds: string[]; choices: CombineChoices }) => api.saveCombine(itemIds, choices),
    onMutate: ({ itemIds, choices }) => {
      const prev = qc.getQueryData<CombineView>(key);
      if (prev) {
        const byId = new Map(prev.items.map((i) => [i.id, i]));
        const items = itemIds.flatMap((id) => byId.get(id) ?? []);
        qc.setQueryData<CombineView>(key, { ...prev, ...choices, items, total: items.reduce((s, i) => s + i.media.duration, 0) });
      }
      return prev;
    },
    onError: (_e, _v, prev) => prev && qc.setQueryData(key, prev),
    onSuccess: (v) => qc.setQueryData(key, v),
  });
  const add = useAddVideos();
  const clear = useMutation({ meta: { errorTitle: "Couldn't start a new combination" }, mutationFn: api.clearCombine, onSuccess: (v) => qc.setQueryData(key, v) });
  const dismiss = useMutation({ meta: { errorTitle: "Couldn't reopen the list" }, mutationFn: api.dismissCombined, onSuccess: (v) => qc.setQueryData(key, v) });
  const setJob = (job: CombineJob) => qc.setQueryData<CombineView>(key, (v) => (v ? { ...v, job } : v));
  const start = useMutation({ meta: { errorTitle: "Couldn't combine the videos" }, mutationFn: api.startCombine, onSuccess: setJob,
    onError: () => qc.invalidateQueries({ queryKey: key }) });
  const cancel = useMutation({ meta: { errorTitle: "Couldn't cancel" }, mutationFn: api.cancelCombine });
  const trash = useMutation({ meta: { errorTitle: "Couldn't move the file to the Trash" }, mutationFn: api.trashCombined, onSuccess: setJob });

  const view = query.data;
  const update = (patch: Partial<CombineChoices> & { itemIds?: string[] }) => {
    if (!view) return;
    const { itemIds = view.items.map((i) => i.id), ...rest } = patch;
    save.mutate({ itemIds, choices: { name: view.name, size: view.size, transition: view.transition, ...rest } });
  };
  return { query, view, update, add, clear, dismiss, start, cancel, trash };
}
