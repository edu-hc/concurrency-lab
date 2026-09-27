#!/usr/bin/env python3
"""Generate comparison charts from concurrency-lab result CSVs.

Usage:
    python3 scripts/generate_charts.py --battery results/2026-09-27_110706.csv \
        --kafka results/2026-09-27_195524.csv --out results/charts

Pass --battery, --kafka, or both, depending on which charts you want:

  --battery  a full-battery CSV (scenarios named "B1 - IO - ...", "B2 - CPU - ...", ...)
             produces latency_gap.png and throughput_io_vs_cpu.png
  --kafka    a kafka-comparison CSV (scenarios named "<strategy> - InMemory" /
             "<strategy> - Kafka") produces inmemory_vs_kafka.png

Requires matplotlib and pandas: pip install -r scripts/requirements.txt
"""
import argparse
import os

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np
import pandas as pd

BLUE = "#4C72B0"
RED = "#C44E52"
ORANGE = "#DD8452"

plt.rcParams.update({
    "font.size": 12,
    "figure.facecolor": "white",
    "axes.facecolor": "white",
    "savefig.facecolor": "white",
})


def despine(ax):
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)


def save(fig, out_dir, filename):
    fig.tight_layout()
    path = os.path.join(out_dir, filename)
    fig.savefig(path, dpi=300)
    plt.close(fig)
    print(f"saved {path}")


def chart_latency_gap(battery, out_dir):
    b1_io = battery[battery["name"].str.startswith("B1 - IO")].copy()
    if b1_io.empty:
        print("skipping latency_gap.png: no 'B1 - IO' scenarios in battery CSV")
        return

    b1_io["strategy"] = b1_io["name"].str.replace("B1 - IO - ", "", regex=False)
    b1_io["p50_latency_ms"] = b1_io["p50_latency_us"] / 1000.0
    b1_io["p50_e2e_ms"] = b1_io["p50_e2e_us"] / 1000.0

    x = np.arange(len(b1_io))
    width = 0.38

    fig, ax = plt.subplots(figsize=(12, 7))
    ax.bar(x - width / 2, b1_io["p50_latency_ms"], width, label="Processing Latency (P50)", color=BLUE)
    ax.bar(x + width / 2, b1_io["p50_e2e_ms"], width, label="End-to-End Latency (P50)", color=RED)

    ax.set_yscale("log")
    ax.set_ylabel("Latency (ms, log scale)")
    ax.set_title("Processing Latency vs End-to-End Latency (IO 5ms)")
    ax.set_xticks(x)
    ax.set_xticklabels(b1_io["strategy"], rotation=40, ha="right")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(lambda v, _: f"{v:g}"))
    ax.grid(axis="y", which="major", linestyle="-", alpha=0.3)
    ax.grid(axis="y", which="minor", linestyle="-", alpha=0.12)
    ax.legend()
    despine(ax)

    save(fig, out_dir, "latency_gap.png")


def chart_throughput_io_vs_cpu(battery, out_dir):
    b1 = battery[battery["name"].str.startswith("B1 - IO")].copy()
    b2 = battery[battery["name"].str.startswith("B2 - CPU")].copy()
    if b1.empty or b2.empty:
        print("skipping throughput_io_vs_cpu.png: need both 'B1 - IO' and 'B2 - CPU' scenarios")
        return

    b1["strategy"] = b1["name"].str.replace("B1 - IO - ", "", regex=False)
    b2["strategy"] = b2["name"].str.replace("B2 - CPU - ", "", regex=False)

    common = sorted(set(b1["strategy"]) & set(b2["strategy"]),
                     key=lambda s: list(b1["strategy"]).index(s))

    b1_common = b1.set_index("strategy").loc[common]
    b2_common = b2.set_index("strategy").loc[common]

    x = np.arange(len(common))
    width = 0.38

    fig, ax = plt.subplots(figsize=(12, 7))
    ax.bar(x - width / 2, b1_common["throughput"], width, label="IO-bound", color=BLUE)
    ax.bar(x + width / 2, b2_common["throughput"], width, label="CPU-bound", color=ORANGE)

    ax.set_ylabel("events/s")
    ax.set_title("Throughput: IO-bound vs CPU-bound (5ms workload)")
    ax.set_xticks(x)
    ax.set_xticklabels(common, rotation=40, ha="right")
    ax.grid(axis="y", linestyle="-", alpha=0.3)
    ax.legend()
    despine(ax)

    save(fig, out_dir, "throughput_io_vs_cpu.png")


def split_template(name):
    if name.endswith(" - InMemory"):
        return name[: -len(" - InMemory")], "InMemory"
    if name.endswith(" - Kafka"):
        return name[: -len(" - Kafka")], "Kafka"
    raise ValueError(f"unexpected scenario name (want '<strategy> - InMemory' or '<strategy> - Kafka'): {name}")


def chart_inmemory_vs_kafka(kafka, out_dir):
    kafka = kafka.copy()
    kafka[["strategy", "template"]] = kafka["name"].apply(lambda n: pd.Series(split_template(n)))

    groups = sorted(kafka["strategy"].unique(), key=list(kafka["strategy"]).index)

    pivot_throughput = kafka.pivot(index="strategy", columns="template", values="throughput").loc[groups]
    pivot_e2e_ms = (kafka.pivot(index="strategy", columns="template", values="p50_e2e_us") / 1000.0).loc[groups]

    if "InMemory" not in pivot_throughput or "Kafka" not in pivot_throughput:
        print("skipping inmemory_vs_kafka.png: need both InMemory and Kafka scenarios")
        return

    x = np.arange(len(groups))
    width = 0.18

    fig, ax1 = plt.subplots(figsize=(12, 7))
    ax2 = ax1.twinx()

    ax1.bar(x - 1.5 * width, pivot_throughput["InMemory"], width,
            label="Throughput (InMemory)", color=BLUE)
    ax1.bar(x - 0.5 * width, pivot_throughput["Kafka"], width,
            label="Throughput (Kafka)", color=ORANGE)

    ax2.bar(x + 0.5 * width, pivot_e2e_ms["InMemory"], width,
            label="P50 E2E (InMemory)", color=BLUE, hatch="//", edgecolor="white", alpha=0.85)
    ax2.bar(x + 1.5 * width, pivot_e2e_ms["Kafka"], width,
            label="P50 E2E (Kafka)", color=ORANGE, hatch="//", edgecolor="white", alpha=0.85)

    ax2.set_yscale("log")
    ax1.set_ylabel("Throughput (events/s)")
    ax2.set_ylabel("P50 End-to-End Latency (ms, log scale)")
    ax1.set_title("InMemory vs Kafka: Cost of Messaging Infrastructure")
    ax1.set_xticks(x)
    ax1.set_xticklabels(groups)

    ax1.grid(axis="y", linestyle="-", alpha=0.3)
    despine(ax1)
    ax2.spines["top"].set_visible(False)

    handles1, labels1 = ax1.get_legend_handles_labels()
    handles2, labels2 = ax2.get_legend_handles_labels()
    ax1.legend(handles1 + handles2, labels1 + labels2, loc="upper center",
               bbox_to_anchor=(0.5, -0.12), ncol=4, frameon=False)

    save(fig, out_dir, "inmemory_vs_kafka.png")


def main():
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("--battery", help="CSV from a full battery run (blocks B1-B5)")
    parser.add_argument("--kafka", help="CSV from a kafka-comparison run (InMemory/Kafka pairs)")
    parser.add_argument("--out", default="results/charts", help="output directory (default: results/charts)")
    args = parser.parse_args()

    if not args.battery and not args.kafka:
        parser.error("pass at least one of --battery or --kafka")

    os.makedirs(args.out, exist_ok=True)

    if args.battery:
        battery = pd.read_csv(args.battery)
        chart_latency_gap(battery, args.out)
        chart_throughput_io_vs_cpu(battery, args.out)

    if args.kafka:
        kafka = pd.read_csv(args.kafka)
        chart_inmemory_vs_kafka(kafka, args.out)


if __name__ == "__main__":
    main()
