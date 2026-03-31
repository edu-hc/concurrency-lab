package event

import (
	"time"

	"github.com/google/uuid"
)

type WorkloadType int

const (
	CPU WorkloadType = iota
	IO
)

type Currency int

const (
	BRL Currency = iota
	USD
	EUR
)

type Event struct {
	ID           string
	CreatedAt    time.Time
	Amount       int64
	Currency     Currency
	Sender       string
	Receiver     string
	WorkloadType WorkloadType
}

func NewEvent(amount int64, currency Currency, sender, receiver string, wt WorkloadType) Event {
	return Event{
		ID:           uuid.New().String(),
		CreatedAt:    time.Now(),
		Amount:       amount,
		Currency:     currency,
		Sender:       sender,
		Receiver:     receiver,
		WorkloadType: wt,
	}
}