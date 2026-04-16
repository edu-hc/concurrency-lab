# concurrency-lab

Ambiente experimental para análise de estratégias de concorrência em Go, desenvolvido como projeto open source.

## O que é

Um framework para testar e comparar diferentes mecanismos de concorrência (worker pool, goroutines on-demand, batching) aplicados ao processamento de eventos de pagamento em larga escala. O objetivo é medir e analisar throughput, latência e contenção sob diferentes cargas e cenários.

## Estrutura

```
cmd/runner/       → ponto de entrada
internal/
  event/          → definição do evento de pagamento
  strategy/       → estratégias de concorrência
  workload/       → simulação de trabalho (CPU, I/O)
  collector/      → coleta e agregação de métricas
  scenario/       → configuração de experimentos
  template/       → contextos de execução (in-memory, Kafka, blockchain)
results/          → saída dos experimentos
```

## Status

Em desenvolvimento — fase de definição do MVP.
