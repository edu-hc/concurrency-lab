# concurrency-lab

Experimental environment for analyzing Go concurrency strategies.

## What it is

A framework for testing and comparing concurrency mechanisms applied to large-scale payment event processing. The focus is measuring and analyzing throughput, latency (average and percentiles), and contention under different loads and scenarios.

## Architecture

The system is composed of eight components:

- **Event** — payment event struct (UUID, amount in cents, currency, sender, receiver, workload type)
- **Strategy** — concurrency interface that receives events via channel and processes them with a workload function
- **Workload** — configurable functions that simulate CPU-bound and I/O-bound work
- **Collector** — thread-safe inline metrics collection (throughput, processing and end-to-end latency, p50/p95/p99 percentiles)
- **Scenario** — declarative struct with experiment parameters
- **Template** — infrastructure context where the strategy runs (in-memory or Kafka)
- **Config** — loads scenario batteries from TOML files, applying defaults and instantiating strategies via a factory
- **Exporter** — writes results to timestamped CSV files

## Strategies implemented

- **Worker Pool** — N fixed goroutines consuming from a shared channel
- **On-Demand** — one goroutine per event, unbounded
- **On-Demand Limited** — one goroutine per event, bounded by a semaphore
- **Batching** — accumulates N events, processes the batch in parallel, waits for the batch to finish
- **Contention Mutex** — shared balances map guarded by a single `sync.Mutex`, to make lock contention visible
- **Contention RWMutex** — same idea, guarded by a `sync.RWMutex` (read-then-write per event)
- **Sharding** — events routed by `hash(sender) % N` to independent, lock-free per-shard maps
- **Pipeline** — three sequential stages (validate → process → record), each with its own worker pool
- **Backpressure** — bounded internal channel; drops events (and records the failure) instead of blocking when full

## Templates implemented

- **InMemory** — generates events in memory and streams them to the strategy through a channel, respecting `rate_per_second` via a ticker
- **Kafka** — creates a single-partition topic exclusive to each scenario run (`topic-<unix_nano>`), publishes all events to it, then consumes them back into the strategy; the topic is deleted when the run finishes

## Structure

```
cmd/runner/          → CLI entry point (urfave/cli)
internal/
  event/             → payment event definition
  strategy/          → concurrency interface and implementations
  workload/          → simulated work (CPU, I/O)
  collector/         → metrics collection and aggregation
  scenario/          → experiment configuration
  template/          → execution contexts (in-memory, Kafka)
  config/            → TOML config loading and strategy factory
  exporter/          → result export (CSV)
presets/             → example TOML scenario batteries
results/             → experiment output (gitignored)
docker-compose.yml   → local Kafka broker (KRaft mode, single node)
```

## How to run

Start Kafka if any scenario uses the `kafka` template:

```bash
docker compose up -d kafka
```

Run a scenario battery from a TOML preset:

```bash
go run ./cmd/runner run --preset presets/io-comparison.toml
```

Flags on `run`:

- `--preset` (required) — path to the TOML preset file
- `--events` — overrides `total_events` for every scenario
- `--rate` — overrides `rate_per_second` for every scenario
- `--timeout` — overrides the per-scenario timeout (e.g. `"120s"`)
- `--output` — output directory for the exported CSV (default `results`)

Results are printed to the terminal and exported as CSV in the output directory. See `presets/` for ready-made batteries: `io-comparison.toml`, `contention.toml`, `battery-full.toml` (the full 49-scenario comparison), and `kafka-comparison.toml` (InMemory vs Kafka).

### Charts

`scripts/generate_charts.py` turns an exported CSV into comparison charts (PNG, 300 DPI). It's not run automatically — regenerate charts on demand from whichever CSV you just produced:

```bash
pip install -r scripts/requirements.txt
python3 scripts/generate_charts.py --battery results/<battery-run>.csv --kafka results/<kafka-run>.csv --out results/charts
```

`--battery` produces `latency_gap.png` and `throughput_io_vs_cpu.png` (needs `B1 - IO - ...` / `B2 - CPU - ...` scenarios, e.g. from `battery-full.toml`); `--kafka` produces `inmemory_vs_kafka.png` (needs `<strategy> - InMemory` / `<strategy> - Kafka` scenarios, e.g. from `kafka-comparison.toml`). Pass either or both. Charts land in `results/`, which is gitignored — the script is checked in, its output isn't.

## Preliminary results

Numbers below are from a rerun after the methodology fixes: batched generator ticks, drops tracked separately from successes, and (for the InMemory-vs-Kafka comparison) `--repeat 3` reporting the median with IQR in parentheses. The full battery (`battery-full.toml`) was run once — see the note on repetition at the end of this section.

### Worker pool sizing and processing vs end-to-end latency (IO, 5ms workload, 100k events @ 50k/s)

- Processing latency stays flat at ~5ms for every strategy **except Pipeline**, which calls the workload function once per stage — three times per event — and measures elapsed time across all three stages (including any inter-stage queueing in the stage buffers), not a single `workFn` call: 18.6ms avg on this run. Its "processing latency" isn't measuring the same thing as everyone else's and shouldn't be compared directly to it.
- End-to-end latency is where strategies diverge by orders of magnitude. Worker Pool(4) and Pipeline(4/stage) never finish 100k events within the 120s timeout (~743-744 events/s, ~89-89.3k events processed when the clock ran out) — their throughput ceiling can't keep up with the 50k events/s arrival rate, so E2E accumulates into tens of seconds (avg ~48s for both).
- On-Demand is the standout now that the generator can actually push 50k/s: with nothing throttling it, it reached **44,774 events/s** — essentially unconstrained, since goroutines blocked on a 5ms I/O wait don't compete for CPU (by Little's Law, L = throughput × latency ≈ 44,774 × 5.5ms ≈ 246 goroutines in flight at once, not an unreasonable number to park). Every bounded strategy (Worker Pool, Contention Mutex/RWMutex, Sharding, Batching(10), OnDemand Limited(10)) converges on ~1,700-2,200 events/s regardless of its specific synchronization mechanism — with a 5ms hold time, *how* you bound concurrency barely matters, only *whether* you do.
- Backpressure (buf=0/10/100) processes only ~3,660-3,813 of the 100k events successfully and drops the rest (~96,200-96,340) — `events_dropped` now makes this explicit instead of it hiding inside a deflated p50. The events that *do* get processed show clean, undistorted latency (p50 ~5.7ms, matching the workload), because drops no longer pull the percentile down.

### IO-bound vs CPU-bound (5ms workload)

- Most bounded strategies land within ~10% of each other whether the workload is IO or CPU-bound (e.g. Worker Pool(10): 1,864 vs 1,972 events/s) — with a 5ms hold time, the lock/semaphore itself is held for a negligible fraction of that time, so contention mechanism barely matters. (Contention Mutex and Contention RWMutex both hold their lock only around the map update, not around `workFn` — a variant holding the lock across part of the simulated work would make contention costs visible instead of hidden.)
- **On-Demand under CPU is the sharpest divergence in the whole battery.** Under IO it's unconstrained (44,774 events/s, ~5.5ms avg latency, matching the workload exactly). Under CPU, unbounded goroutines busy-spin instead of parking: this single battery run shows throughput collapsing to 2,331 events/s while avg latency balloons to 57.2ms, 11.4x the nominal 5ms hold. This machine has 12 cores; by Little's Law, 2,331 events/s at 57.2ms implies ~133 goroutines competing for those 12 — an 11.1x oversubscription ratio that lines up with the 11.4x latency inflation within 3%, a real cross-check rather than just a plausible story. A separate `--repeat 3` check on this same scenario confirms throughput is reproducible (2,392 events/s median, IQR under 6) but shows avg latency itself varies a lot run to run (26-57ms across the sample) and P50 end-to-end latency even more so (IQR ~5.7s on a ~6.3s median, ~90% spread) — CPU saturation makes the generator alternate between bursting into the channel buffer and blocking on it once full, so how bunched-up arrivals are varies a lot between runs even though the strategy's steady-state throughput doesn't. Treat any single-run latency number for this scenario as illustrative, not precise.
- Pipeline under CPU shows the same effect compounding with its own 3x-work multiplier: avg processing latency of 102.6ms (vs. 18.6ms under IO) — the three worker pools per stage are themselves competing for CPU on top of each stage's own busy-spin cost.
- Batching(100) is the clearest *good* IO/CPU split: 17,644 events/s under IO vs. 2,189 under CPU — batching amortizes overhead well when the workload is I/O-bound and barely helps when it's CPU-bound.

### Contention and worker-count scaling (100µs workload)

At a much shorter 100µs hold time (`battery-full.toml`'s B3/B4 blocks), the fixed generator can finally push enough load to make worker-count scaling visible instead of masking it: Contention Mutex/RWMutex/Sharding all roughly triple their throughput going from 10 to 50 workers (e.g. Contention Mutex: 13,192 → 46,505 events/s under IO), and Sharding's lock-free design doesn't show a consistent edge over the mutex-based strategies at this scale — the hash/dispatch overhead it trades locking for isn't free either.

### InMemory vs Kafka (10k events @ 5k/s, median of 3 runs)

- Kafka throughput comes out well below in-memory across the board, and the gap is now much larger than a pre-fix run suggested — because the fixed generator lets the in-memory side reach much closer to its real ceiling, while Kafka's ceiling doesn't move: Worker Pool IO 1,787 → 855 events/s (-52%), On-Demand IO 4,464 → 1,101 events/s (-75%), Worker Pool CPU 1,996 → 872 events/s (-56%).
- Kafka's throughput looks close to workload-independent: Worker Pool via Kafka lands within ~2% of itself under IO vs. CPU (855 vs. 872 events/s), despite the same strategy differing by ~12% in-memory. That's consistent with (not proof of) the single-goroutine, one-message-at-a-time `ReadMessage` loop in `consume()` being the real throughput cap rather than the workload — measuring the produce and consume phases separately would confirm it.
- End-to-end latency goes from single-digit milliseconds (in-memory, unsaturated) to seconds via Kafka (2.4-5.1s median) — several orders of magnitude worse. This is structural: the current Kafka template's production and consumption phases run sequentially (all events are published, the writer is closed, *then* consumption starts), not concurrently like the in-memory template.
- **Run-to-run variance is real and strategy-dependent.** Most Kafka scenarios were tight across 3 repeats (throughput IQR under 1.3 events/s), but a single ad hoc run of CPU Worker Pool Kafka (not the number reported above, which is the 3-run median) took 89s instead of the usual ~11s for identical input, with per-event latency and error/drop counts completely normal throughout — the slowdown was entirely in how fast Kafka handed messages to the consumer, not in anything the strategy or collector did. That run alone would have been a badly misleading data point; it's exactly the kind of noise `--repeat` exists to average out rather than hide.

### On repetition

Only the InMemory-vs-Kafka comparison above used `--repeat`; the 49-scenario `battery-full.toml` run was a single pass (it already takes ~25 minutes once; repeating it 5x for a real confidence interval would take over two hours). Treat the battery numbers as a single, reasonably informative sample, not a rigorous estimate — rerun with `--repeat` if a specific comparison needs a real interval.

As a spot check, four battery scenarios chosen for being either surprising or load-bearing in the analysis above (On-Demand under IO and CPU, Backpressure buf=0 under IO, Contention Mutex(50) at 100µs) were rerun with `--repeat 3`. Throughput held up well in all four (IQR within ~0.2-2% of the median, matching the single-run battery figures within a few percent) — the one exception is CPU On-Demand's latency numbers specifically, noted inline above. That's a reasonable basis for trusting throughput comparisons drawn from the single-pass battery data, but not a substitute for actually repeating a scenario before leaning on its latency numbers.
