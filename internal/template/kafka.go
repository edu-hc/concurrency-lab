package template

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	kafka "github.com/segmentio/kafka-go"

	"concurrency-lab/internal/collector"
	"concurrency-lab/internal/event"
	"concurrency-lab/internal/scenario"
)

// Kafka is a Template that publishes generated events to a Kafka topic
// (production phase) and then feeds the strategy from a consumer reading
// that same topic (consumption phase). Each Execute run gets its own
// exclusive, single-partition topic so scenarios never interfere with
// each other; the topic is deleted once the run finishes.
type Kafka struct {
	brokers []string
	topic   string
}

func NewKafka(brokers []string, topic string) *Kafka {
	return &Kafka{brokers: brokers, topic: topic}
}

func (k *Kafka) Execute(ctx context.Context, scn scenario.Scenario, col collector.Collector) error {
	topicName := fmt.Sprintf("%s-%d", k.topic, time.Now().UnixNano())

	if err := k.createTopic(topicName); err != nil {
		return err
	}
	defer k.deleteTopic(topicName)

	if err := k.produce(ctx, scn, topicName); err != nil {
		return err
	}

	events := k.consume(ctx, scn.TotalEvents, scn.RatePerSecond, topicName)

	workFn := workloadFunc(scn.WorkloadType, scn.WorkloadDuration)

	start := time.Now()
	err := scn.Strategy.Run(ctx, events, workFn, col)
	col.SetTotalDuration(time.Since(start))

	return err
}

// createTopic creates a single-partition, unreplicated topic exclusive to
// one scenario run, and polls until its metadata is available on the broker.
func (k *Kafka) createTopic(topicName string) error {
	conn, err := kafka.Dial("tcp", k.brokers[0])
	if err != nil {
		return fmt.Errorf("dialing broker to create topic %s: %w", topicName, err)
	}
	defer conn.Close()

	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             topicName,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}); err != nil {
		return fmt.Errorf("creating topic %s: %w", topicName, err)
	}

	// Poll until the topic metadata is available on the broker.
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		partitions, err := conn.ReadPartitions(topicName)
		if err == nil && len(partitions) > 0 {
			return nil
		}
	}

	return fmt.Errorf("topic %s not ready after 15s", topicName)
}

// deleteTopic removes the scenario-exclusive topic created by createTopic.
func (k *Kafka) deleteTopic(topicName string) error {
	conn, err := kafka.Dial("tcp", k.brokers[0])
	if err != nil {
		return fmt.Errorf("dialing broker to delete topic %s: %w", topicName, err)
	}
	defer conn.Close()

	if err := conn.DeleteTopics(topicName); err != nil {
		return fmt.Errorf("deleting topic %s: %w", topicName, err)
	}

	return nil
}

// produce generates scn.TotalEvents events, respecting scn.RatePerSecond
// via a ticker, and publishes each as JSON to the Kafka topic.
func (k *Kafka) produce(ctx context.Context, scn scenario.Scenario, topicName string) error {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(k.brokers...),
		Topic:        topicName,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    1,
		BatchTimeout: 10 * time.Millisecond,
		// Without this, the writer resolves partitions from kafka-go's shared,
		// client-side metadata cache and never rechecks it once a topic is
		// marked unknown there (see kafka.Writer.partitions /
		// Transport.roundTrip). Since every scenario run creates a brand new,
		// uniquely-named topic, that cache will not have it yet, and the
		// writer would otherwise fail every publish with "Unknown Topic Or
		// Partition" until the shared transport's background metadata
		// refresh (every ~6s) happens to catch up. This flag does not cause
		// the broker to auto-create anything here (createTopic already
		// created the topic deterministically) — it only makes the client
		// force a live metadata lookup on a cache miss instead of trusting
		// the stale cache.
		AllowAutoTopicCreation: true,
	}
	defer writer.Close()

	ticker := time.NewTicker(time.Second / time.Duration(scn.RatePerSecond))
	defer ticker.Stop()

	sent := 0
	for sent < scn.TotalEvents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ev := event.NewEvent(
				int64(100+rand.Intn(99901)),
				event.Currency(rand.Intn(3)),
				randomUser(),
				randomUser(),
				scn.WorkloadType,
			)

			payload, err := json.Marshal(ev)
			if err != nil {
				return fmt.Errorf("marshaling event %s: %w", ev.ID, err)
			}

			if err := writer.WriteMessages(ctx, kafka.Message{Value: payload}); err != nil {
				return fmt.Errorf("publishing event %s: %w", ev.ID, err)
			}
			sent++
		}
	}

	return nil
}

// consume reads totalEvents messages from the Kafka topic, deserializes
// them back into event.Event, and streams them into the returned channel,
// closing it once totalEvents messages have been read, the context is
// cancelled, or the reader errors out. The topic is exclusive to this run
// and has a single partition, so no consumer group is needed.
func (k *Kafka) consume(ctx context.Context, totalEvents int, bufferSize int, topicName string) <-chan event.Event {
	events := make(chan event.Event, bufferSize)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     k.brokers,
		Topic:       topicName,
		Partition:   0,
		StartOffset: kafka.FirstOffset,
	})

	go func() {
		defer close(events)
		defer reader.Close()

		received := 0
		for received < totalEvents {
			if ctx.Err() != nil {
				return
			}

			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				return
			}

			var ev event.Event
			if err := json.Unmarshal(msg.Value, &ev); err != nil {
				continue
			}

			select {
			case events <- ev:
				received++
			case <-ctx.Done():
				return
			}
		}
	}()

	return events
}
