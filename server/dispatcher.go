package server

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/oklahomer/go-sarah"
	"golang.org/x/net/context"
)

// Dispatcher internally encapsulates given content into proper form of sarah.Output and sends this to given destination.
type Dispatcher interface {
	// Dispatch sends given content to given destination.
	Dispatch(*Destination, interface{}) error

	// DispatchTransactional sends given content to given destination and blocks till all devices returns responses.
	// This gives up and returns empty slice of *Response when responses are not given within time.Duration.
	DispatchTransactional(*Destination, interface{}, time.Duration) ([]*Response, error)
}

type dispatcher struct {
	bot sarah.Bot
}

var _ Dispatcher = (*dispatcher)(nil)

// NewDispatcher creates and returns new Dispatcher instance.
func NewDispatcher(bot sarah.Bot) Dispatcher {
	return &dispatcher{
		bot: bot,
	}
}

func (d *dispatcher) Dispatch(destination *Destination, content interface{}) error {
	output := NewOutput(destination, content)
	d.bot.SendMessage(context.TODO(), output)
	return nil
}

func (d *dispatcher) DispatchTransactional(destination *Destination, content interface{}, timeout time.Duration) ([]*Response, error) {
	transactionID := strconv.Itoa(idDispenser.next())
	output, responseCh := NewTransactionalOutput(transactionID, destination, content)
	d.bot.SendMessage(context.TODO(), output)

	select {
	case responses := <-responseCh:
		return responses, nil

	case <-time.NewTimer(timeout).C:
		return []*Response{}, fmt.Errorf("timeout")

	}
}

var idDispenser = newTransactionID()

type transactionIDDispenser struct {
	id    int
	mutex *sync.Mutex
}

func newTransactionID() *transactionIDDispenser {
	return &transactionIDDispenser{
		id:    0,
		mutex: &sync.Mutex{},
	}
}

func (t *transactionIDDispenser) next() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.id++
	return t.id
}
