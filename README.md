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

> The numbers below predate three methodology fixes: the in-memory generator silently sustaining far less than its nominal rate at high frequencies (fixed by batching events per tick instead of one per tick), `Backpressure` drops contaminating the success percentiles (fixed by `Collector.RecordDropped`, tracked separately from `Record`), and single, unrepeated runs with no variance reported (fixed by `--repeat`, reporting median + IQR). Treat specific figures here as illustrative until this section is refreshed against a rerun with the fixes in place — see `TODO.md`.

### Worker pool sizing and processing vs end-to-end latency (IO, 5ms workload, 100k events @ 50k/s)

- Processing latency stays flat at ~5-6ms for every strategy **except Pipeline**, which calls the workload function once per stage — three times per event — and measures elapsed time across all three stages (including any inter-stage queueing in the stage buffers), not a single `workFn` call. Its "processing latency" isn't measuring the same thing as everyone else's and shouldn't be compared directly to it.
- End-to-end latency is where strategies diverge by orders of magnitude: undersized pools (Worker Pool with 4 workers, Pipeline at 4 workers/stage) accumulate tens of seconds of queueing latency because their max throughput (~800 events/s) can't keep up with the 50k events/s arrival rate.
- On-Demand, On-Demand Limited (50), and the Backpressure variants are the only strategies that keep end-to-end latency close to processing latency — Backpressure achieves this by dropping events under load instead of queueing them (tens of thousands of drops at these rates, now tracked in `Dropped`/`events_dropped` instead of leaking into the latency percentiles).

### IO-bound vs CPU-bound throughput (5ms workload)

- Throughput is similar between IO and CPU workloads for most strategies — with a 5ms hold time, the mutex/semaphore itself is held for a negligible fraction of that time, so contention barely matters. (Both Contention Mutex and Contention RWMutex hold their lock only around the map update, not around `workFn` — a variant holding the lock across part of the simulated work would make contention costs visible instead.)
- Batching(100) is the clearest exception: throughput is nearly 2x higher under IO than CPU, since batching amortizes overhead better when the workload is I/O-bound.

### InMemory vs Kafka (10k events @ 5k/s)

- Kafka reduces throughput by roughly 11-56% compared to the in-memory template, varying a lot by strategy.
- End-to-end latency goes from single-digit milliseconds (in-memory) to several seconds via Kafka — several orders of magnitude worse. Part of this is structural: the current Kafka template's production and consumption phases run sequentially (all events are published, the writer is closed, *then* consumption starts), not concurrently like the in-memory template. Separately, Kafka's throughput ceiling looks close to workload-independent (Worker Pool comes out within a couple percent of itself under IO vs. CPU, despite the two workloads differing substantially in-memory) — consistent with, but not proof of, the single-goroutine `ReadMessage` consumer loop being the real cap rather than the workload itself. Measuring the produce and consume phases separately would confirm this.
