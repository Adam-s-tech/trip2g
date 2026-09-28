# Pool memory report — 2026-09-28

## Decision

There is no demonstrated need for a broad memory optimization or emergency tuning at present. The six captured containers use 87–191 MiB of `memory.current` against a 512 MiB limit. Most private memory is explained by Go's loaded notes, syntax trees, vectors, and allocator/runtime overhead. SQLite and Bleve mappings are small in these snapshots.

Keep the current configuration until workload measurements justify a change. If fleet density becomes a constraint, chunk text is a reasonable first retained-memory target. If reliability near the container limit becomes a concern, investigate transient loading/reload peaks first. Neither removing ASTs nor compressing every HTML field is an immediate priority.

This conclusion applies to the captured workloads. Processes were only 22–23 minutes old; a single capture cannot establish long-term stability or rule out a leak. Historical RSS peaks are already substantially higher than current usage.

## Evidence and scope

Only the new capture set was used:

```text
/tmp/claude-1000/-home-alexes-projects2-box-coxswain/6f05cfa9-cc03-4b82-8dea-9beb4d72b0b3/scratchpad/pprof-2026-09-28-full/
```

- Image revision: `c2b7cf62`; binary build ID: `e1059d5a3bcd3b32a4009f5b9a93e112abc41d68`.
- Go version reported by the processes: `go1.26.8`.
- Capture time: approximately 07:00:53–54 UTC; each collection window was 0.104–0.120 seconds.
- Environment: `GOGC=50`, `GOMEMLIMIT=400MiB`; all six cgroups have `memory.max=512MiB`.
- Inputs: before/after Go metrics and process memory, heap profile with `gc=1`, textual heap/MemStats, full `smaps`, cgroup accounting, environment, and goroutines.
- The old `pprof-2026-09-28/` profiles from `ab2aede7`, including the former `epicbeauvoir` observation, are excluded. They are not a before/after baseline for these processes.
- `01_meta` reports **data directory size**, not just SQLite database size.

The capture directory also contains `analysis_data.json`, `memory-analysis-ru.txt`, and per-instance `analysis_pprof.txt` with the detailed accounting and pprof output. These are temporary local artifacts; the main findings are preserved below.

## Measured memory

All sizes are MiB, including values that pprof labels `MB`.

| Instance | RSS after GC | PSS after GC | cgroup current | HeapAlloc | HeapInuse | Historical MaxRSS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| justcoetzee | 237.3 | 187.4 | 191.1 | 138.5 | 154.8 | 438.1 |
| daringmunro | 191.5 | 141.9 | 146.3 | 83.4 | 102.5 | 303.7 |
| grandsteinbeck | 179.6 | 129.1 | 148.2 | 74.3 | 89.2 | 352.7 |
| clevercalvino | 138.6 | 89.0 | 93.6 | 57.6 | 72.9 | 233.3 |
| clevergordimer | 137.7 | 87.6 | 92.2 | 58.1 | 68.5 | 242.4 |
| honestrilke | 136.2 | 86.1 | 87.3 | 46.1 | 55.2 | 263.5 |

`HeapAlloc` and `HeapInuse` come from `21_go_metrics.after_gc.txt`. `MaxRSS` comes from the textual profile and agrees with `VmHWM` in process status. It is the process's historical RSS maximum, not the cgroup maximum. The time and operation responsible for each peak are unknown.

In particular, **438 MiB MaxRSS on justcoetzee is a reason to measure peaks before reducing limits**, but subtracting it from the 512 MiB cgroup limit does not calculate the actual margin to OOM. RSS and cgroup accounting differ. `memory.peak` and `memory.events` were not included in this collection.

### Shared binary pages explain much of the RSS/PSS difference

`/trip2g` contributes approximately 49–51 MiB RSS per process, but only 0.9–1.4 MiB PSS. Most of those resident pages are shared. Summing process RSS therefore counts the same physical pages repeatedly.

`Shared_Clean` is not exclusively the binary: on justcoetzee, about 48.6 MiB belongs to `/trip2g` and 3.5 MiB to SQLite mappings. Multiple mappings do not by themselves imply that different tenants share a database.

Use cgroup accounting for container limits, and host/parent-cgroup measurements together with PSS when planning fleet capacity. PSS does not include all filesystem cache or kernel memory; cgroup charging of shared pages is not proportional like PSS. See [Linux process accounting](https://docs.kernel.org/filesystems/proc.html) and [cgroup v2 memory accounting](https://docs.kernel.org/admin-guide/cgroup-v2.html).

### Most private resident memory is explicitly identified as Go memory

The full maps contain `[anon: Go: heap]` and named runtime mappings. Together they account for 176.3 MiB on justcoetzee, 132.9 MiB on daringmunro, and 78.1 MiB on honestrilke. Across all six processes, only 2.5–2.9 MiB of anonymous memory lies outside the named Go mappings, including private binary pages and other allocations.

`Sys - HeapReleased` is close to total `Anonymous`, within approximately 2.6 MiB. The figures are not expected to match exactly: runtime accounting is not a residency measurement, and collection is sequential.

For justcoetzee, `Sys - HeapReleased = 181.5 MiB` decomposes as:

| Component | MiB |
| --- | ---: |
| HeapAlloc | 138.5 |
| HeapInuse minus HeapAlloc | 16.3 |
| HeapIdle minus HeapReleased | 12.1 |
| Other runtime memory: Sys minus HeapSys | 14.6 |

The second row is an upper bound on unused space within occupied spans, not a guaranteed reclaimable allocation. Entire idle spans not yet released to the OS also account for 22.1 MiB on daringmunro and 25.8 MiB on grandsteinbeck. These allocator reserves are not evidence of a leak.

### SQLite is not the main target in these captures

Resident SQLite mappings, including shared-memory files, total 0.4–3.9 MiB per process. Bleve index mappings total 0.5–5.4 MiB. There is no large resident database mapping to eliminate here. Cgroup file cache can be larger than mapped RSS: grandsteinbeck has 19.2 MiB of `file` accounting, while its mapped search index occupies only about 0.45 MiB RSS.

The argument “modernc SQLite is pure Go, therefore all its memory appears in the Go heap” is incorrect. In the dependencies used by this build, the allocator path is `sqlite3MemMalloc -> libc.Xmalloc -> memory.Allocator -> unix.MmapPtr`, which can allocate outside the Go heap. The relevant versions are `modernc.org/sqlite v1.53.0`, `modernc.org/libc v1.73.4`, and `modernc.org/memory v1.11.0`. Driver-created Go strings and byte slices from `columnText` and `columnBlob` do appear in pprof.

The reason to leave SQLite tuning alone is the measured mapping/private-memory breakdown, not the absence of cgo. The configured `mmap_size` and `cache_size` in [database setup](../../internal/db/setup.go) are allowances, not proof of allocated resident memory.

## What the heap retains

| Instance | Sampled pprof heap | Float32 vectors | Markdown parse subtree | HTML strings | Chunk strings |
| --- | ---: | ---: | ---: | ---: | ---: |
| justcoetzee | 130.9 | 35.6 | 34.5 | 15.2 | 14.9 |
| daringmunro | 83.7 | 21.6 | 17.5 | 6.0 | 6.3 |
| grandsteinbeck | 88.1 | 18.6 | 23.5 | 14.0 | 5.5 |
| clevercalvino | 50.5 | 14.1 | 11.5 | 2.5 | 4.0 |
| clevergordimer | 57.9 | 18.6 | 12.5 | 4.5 | 4.0 |
| honestrilke | 44.2 | 7.0 | 9.0 | 6.1 | 6.5 |

These are sampled allocation estimates with a 512 KiB sampling period, not an exact census or a complete decomposition of HeapAlloc. The Markdown subtree is predominantly AST allocations. HTML is attributed to `bytes.Buffer.String` in rendering paths. Chunk strings are `columnText` allocations within `loadChunks`, mainly content, with paths also returned by the query. Do not add cumulative `loadChunks` to vectors and strings: those allocations are already inside it.

The profile and runtime metrics can disagree materially: grandsteinbeck's sampled heap is 88.1 MiB, while its later HeapAlloc is 74.3 MiB. Collection itself also allocates: TotalAlloc increased by roughly 6–9 MiB between the before/after metrics, including profiling and metric requests. This is not an isolated experiment with GC as the only activity.

Lifetime alloc_space is approximately 269–629 MiB across the six young processes, including initialization. This collection does not establish heavy steady-state GC or gzip allocation pressure. Most goroutine snapshots contain 40 goroutines; grandsteinbeck has 41. Neither count nor one heap snapshot establishes a long-term leak trend.

## Removing chunk text from RAM: moderate complexity

This is a bounded change across the loader, search, and MCP consumers. It is not simply deleting `NoteChunk.Content`.

On justcoetzee, `loadChunks` accounts for about 48.9 MiB: 33.1 MiB of chunk vectors, 14.9 MiB of strings, and 0.9 MiB of structures. Another 2.5 MiB of vector allocations belongs to whole-note embeddings. Loading chunk text on demand targets roughly **15 MiB gross retained allocations** on this instance and **4–6.5 MiB** on the others. Net savings depend on replacement metadata, request allocations, and any cache; RSS savings are not guaranteed to match those numbers.

The vector scoring loop only needs vectors and identity. The complication is what happens after scoring:

| Consumer | Current use of chunk text | Required adaptation |
| --- | --- | --- |
| [Vector retrieval](../../internal/case/sitesearch/retrieve.go) | Snippets and passages for reranking | Rank first, then batch-fetch text for selected candidates; preserve the pre-fusion candidate budget |
| [MCP search payload](../../internal/case/mcp/resolve.go) | Breadcrumb/TOC paths and chunk identity | Load selected text into a request-local lookup |
| MCP BM25 snippet matching | Compares a snippet against all chunks of the returned note | Fetch chunks for the relevant result notes, not just vector winners |
| MCP focused reading | Reads the selected chunk and its neighbors | Fetch a small chunk-index range for the selected note version |
| [Note loader](../../internal/noteloader/loader.go) | Keeps content in the full resident chunk slice | Stop selecting/storing content in the resident-vector loading path |

An implementation should:

1. Keep a resident vector/identity representation separate from fetched text. Remove content from the all-chunks load queries, not merely clear the field after loading it.
2. Add content-only batch queries in [queries.read.sql](../../queries.read.sql), exposed through minimal use-case Env interfaces. The existing `GetNoteVersionChunks` uses `select *`, including embeddings; using it unchanged would re-read unnecessary BLOBs. Existing tables should suffice; no schema migration is inherently required.
3. Carry exact version/chunk identity through retrieval rather than resolving content by current path. Define behavior when embeddings/chunks are regenerated during a request: the same version can be re-chunked, so version ID alone is not a complete generation guarantee. Use a consistent generation or a defined retry/fallback when the referenced chunk changes or disappears.
4. Preserve live/latest isolation and existing authorization. Avoid one database query per candidate or snippet. Start with request-local reuse; add a byte-bounded cross-request cache only if latency measurements justify it, with generation-aware invalidation.
5. Test vector and BM25-only search, reranking, MCP `match_id`/`toc_path`, neighbor windows, and regeneration/reload races. Compare result correctness, query count, search latency, post-GC heap, and peak cgroup memory on a representative corpus.

The vector-ranking part is straightforward; MCP compatibility and consistency during regeneration are most of the work. At the measured sizes, this is worth keeping as an optional density improvement, not scheduling as an urgent fix.

## Storing HTML compressed in memory

### Completed HTTP responses are already cached as gzip

[PageCache](../../internal/pagecache/pagecache.go) stores pre-gzipped complete anonymous page responses. [fillPageCache](../../internal/case/rendernotepage/pagecache.go) compresses the assembled response before storing it. The cache has a 30-second TTL and an 8192-entry cap; that is an entry cap, not a byte budget. Changing its storage to gzip would duplicate existing behavior. Its byte usage is worth measuring under traffic, but these profiles do not identify it as the dominant retained-memory consumer.

### Compressing NoteView HTML is a separate change

`NoteView.HTML`, `FreeHTML`, and `DomainHTML` currently hold HTML fragments used to assemble responses and support other features. Replacing their plain strings with compressed bytes can save memory, but adding compressed copies while retaining all original strings cannot.

Consumers needing decoded HTML include:

- Page bodies, sidebars, free previews, and [custom template views](../../internal/templateviews/note.go).
- [Embedded notes](../../internal/mdloader/link_renderer.go) during rendering.
- MCP full-note/section reads and TOC/snippet extraction.
- GraphQL HTML fields and [change-selector calculation](../../internal/case/calculatechangeselectors/resolve.go), including previous-version HTML.

Those consumers cannot all directly serve the compressed fragment as a finished HTTP response. The ordinary rendering path would decode the fragment, combine it with layouts and other content, and potentially gzip the final response again. Cached anonymous response hits already avoid that work, but cache misses and other consumers would pay decompression/allocation costs.

The measured HTML-string targets are about **15.2 MiB on justcoetzee** and **14.0 MiB on grandsteinbeck**, including **10.3 MiB of domain variants** on the latter. They are much smaller than total process RSS, and compressing them does not remove Markdown source, ASTs, or embeddings.

For scale only: if actual HTML compressed to one quarter of its original size, 15.2 MiB would save approximately 11.4 MiB before metadata and decoded buffers. **That ratio was not measured**; heap profiles contain allocation metadata, not the HTML corpus, so they cannot establish a compression ratio.

A sensible experiment, if density later warrants it:

1. Measure total retained HTML bytes and actual gzip size on a representative note/domain corpus, including small fragments and free previews.
2. Prototype compression of cold domain variants first, behind a consistent HTML-access abstraction. Keep immutable compressed payloads; use request-local decoded reuse or a strictly byte-bounded cache. A permanently decoded copy for every note would erase the benefit.
3. Compress finalized render output only after embed dependencies have consumed it, then release plain references where safe. Include `prevHTML` and other holders when assessing retention.
4. Measure retained heap and peak memory during reload, cache-miss latency, CPU, allocations, and concurrent decompression. Validate templates, embeds, paywall previews, domain links, MCP, GraphQL, and HTML diffs.

This is feasible but touches a wider rendering surface than lazy chunk text. Full HTML compression is not justified by the current memory pressure. A domain-only prototype would be a smaller way to measure the tradeoff before committing to it.

## Where to look next, if a concrete constraint appears

| Trigger | Investigation | Candidate action |
| --- | --- | --- |
| Need more pool instances per host | Measure parent-cgroup/host memory and representative per-instance PSS/current usage | Lazy chunk text; consider cold domain HTML only after measuring compression |
| Reload/startup approaches the container limit | Record `memory.peak`, `memory.events`, and memory time series during startup, large sync, embedding regeneration, and backup | Reduce overlapping old/new snapshots and batch/stream raw embedding reads |
| Suspected leak after long uptime | Compare post-GC profiles after repeated equivalent operations and after hours/day of operation | Investigate the growing allocation stacks, with workload and note/chunk counts held comparable |
| High GC CPU or request allocation rate | Compare allocation deltas and CPU under a fixed workload | Reuse page-cache gzip writers or investigate transient buffers only where measured |

The [loader](../../internal/noteloader/loader.go) currently materializes raw embedding BLOBs before converting them to float32 slices. During reload, old vectors, raw bytes, and replacement vectors can overlap. Batching that conversion is a plausible peak-memory improvement; the recorded MaxRSS does not prove that this path caused the peak.

Do not lower GOGC or schedule forced GC as a substitute for understanding retained data. Current `Sys - HeapReleased` is approximately 83–181 MiB, far below `GOMEMLIMIT=400MiB`; lowering the limit slightly below 400 MiB would not constrain the present steady state. A Go memory limit also does not cover the entire cgroup. See the [Go GC guide](https://go.dev/doc/gc-guide).

No application or deployment changes were made for this report. The next useful work is measurement around known peaks, not a broad memory refactor.
