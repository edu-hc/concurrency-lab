# concurrency-lab

Ambiente experimental para análise de estratégias de concorrência em Go.

## O que é

Um framework para testar e comparar diferentes mecanismos de concorrência aplicados ao processamento de eventos de pagamento em larga escala. O foco é medir e analisar throughput, latência (média e percentis) e contenção sob diferentes cargas e cenários.

## Arquitetura

O sistema é composto por seis componentes:

- **Event** — struct de evento de pagamento (UUID, amount em centavos, currency, sender, receiver, workload type)
- **Strategy** — interface de concorrência que recebe eventos via channel e processa com uma workload function
- **Workload** — funções configuráveis que simulam trabalho CPU-bound e I/O-bound
- **Collector** — coleta inline thread-safe de métricas (throughput, latência de processamento e end-to-end, percentis p50/p95/p99)
- **Scenario** — struct declarativo com parâmetros do experimento
- **Template** — contexto de infraestrutura onde a strategy executa (in-memory no momento)

## Strategies implementadas

- **Worker Pool** — N goroutines fixas consumindo de um channel compartilhado
- **On-Demand** — uma goroutine por evento, sem limite
- **On-Demand Limited** — uma goroutine por evento com semáforo limitando concorrência máxima
- **Batching** — acumula N eventos, processa o lote em paralelo, espera o batch terminar

## Estrutura

```
cmd/runner/          → ponto de entrada e execução de cenários
internal/
  event/             → definição do evento de pagamento
  strategy/          → interface e implementações de concorrência
  workload/          → simulação de trabalho (CPU, I/O)
  collector/         → coleta e agregação de métricas
  scenario/          → configuração de experimentos
  template/          → contextos de execução (in-memory)
  exporter/          → exportação de resultados (CSV)
results/             → saída dos experimentos
```

## Como rodar

```bash
go run cmd/runner/main.go
```

Os resultados são impressos no terminal e exportados como CSV em `results/`.

## Resultados preliminares

Experimentos com 10.000 eventos, taxa de 10.000/s e workload de 5ms revelaram:

- Worker Pool escala linearmente com número de workers em I/O-bound
- On-Demand tem throughput superior mas latência de cauda (p99) pior em CPU-bound devido a contenção no scheduler
- Batching tem throughput similar ao Worker Pool de mesmo N, mas E2E pior devido à barreira de sincronização entre batches
- On-Demand Limited com semáforo reduz p99 em CPU comparado ao ilimitado, ao custo de throughput
- Latência de processamento pode parecer saudável enquanto latência end-to-end revela acumulação severa na fila

## Roadmap

- Novas strategies: backpressure, contenção explícita, sharding, pipeline/staged
- Template Kafka
- Template blockchain-inspired
- Exportação JSON

## Status

Em desenvolvimento ativo — fase de expansão de strategies e análise experimental.