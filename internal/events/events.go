package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/state"
)

type Kind string

const (
	CheckCompleted   Kind = "check.completed"
	IncidentOpened   Kind = "incident.opened"
	IncidentResolved Kind = "incident.resolved"
)

type Event struct {
	Kind     Kind            `json:"kind"`
	Monitor  string          `json:"monitor"`
	At       time.Time       `json:"at"`
	Result   *check.Result   `json:"result,omitempty"`
	Incident *state.Incident `json:"incident,omitempty"`
}

func FromResult(r check.Result) Event {
	return Event{Kind: CheckCompleted, Monitor: r.Monitor, At: r.At, Result: &r}
}

func FromIncident(i state.Incident) Event {
	e := Event{Kind: IncidentOpened, Monitor: i.Monitor, At: i.Started, Incident: &i}
	if i.Resolved != nil {
		e.Kind, e.At = IncidentResolved, *i.Resolved
	}
	return e
}

type Publisher interface {
	Publish(ctx context.Context, e Event) error
	Close() error
}

type Log struct{ Logger *slog.Logger }

func (l Log) Publish(_ context.Context, e Event) error {
	if e.Kind != CheckCompleted {
		l.Logger.Info("event", "kind", e.Kind, "monitor", e.Monitor, "at", e.At)
	}
	return nil
}

func (Log) Close() error { return nil }

type Kafka struct {
	checks    *kafka.Writer
	incidents *kafka.Writer
}

func NewKafka(brokers []string, checksTopic, incidentsTopic string) *Kafka {
	w := func(topic string) *kafka.Writer {
		return &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.Hash{},
			AllowAutoTopicCreation: true,
			BatchTimeout:           50 * time.Millisecond,
			RequiredAcks:           kafka.RequireOne,
		}
	}
	return &Kafka{checks: w(checksTopic), incidents: w(incidentsTopic)}
}

func (k *Kafka) Publish(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	w := k.checks
	if e.Kind != CheckCompleted {
		w = k.incidents
	}
	return w.WriteMessages(ctx, kafka.Message{Key: []byte(e.Monitor), Value: body, Time: e.At})
}

func (k *Kafka) Close() error {
	err := k.checks.Close()
	if err2 := k.incidents.Close(); err == nil {
		err = err2
	}
	return err
}
