package server

import (
	"testing"
	"time"

	"github.com/oklahomer/go-sarah"
	"golang.org/x/net/context"
)

type DummyBot struct {
	BotTypeValue      sarah.BotType
	RespondFunc       func(context.Context, sarah.Input) error
	SendMessageFunc   func(context.Context, sarah.Output)
	AppendCommandFunc func(sarah.Command)
	RunFunc           func(context.Context, func(sarah.Input) error, func(error))
}

var _ sarah.Bot = (*DummyBot)(nil)

func (bot *DummyBot) BotType() sarah.BotType {
	return bot.BotTypeValue
}

func (bot *DummyBot) Respond(ctx context.Context, input sarah.Input) error {
	return bot.RespondFunc(ctx, input)
}

func (bot *DummyBot) SendMessage(ctx context.Context, output sarah.Output) {
	bot.SendMessageFunc(ctx, output)
}

func (bot *DummyBot) AppendCommand(command sarah.Command) {
	bot.AppendCommandFunc(command)
}

func (bot *DummyBot) Run(ctx context.Context, enqueueInput func(sarah.Input) error, notifyErr func(error)) {
	bot.RunFunc(ctx, enqueueInput, notifyErr)
}

func TestNewDispatcher(t *testing.T) {
	bot := &DummyBot{}

	d := NewDispatcher(bot)

	if d == nil {
		t.Fatal("Dispatcher implementation is not returned.")
	}

	typed, ok := d.(*dispatcher)

	if !ok {
		t.Fatalf("Unexpected type of Dispatcher implementation is returned: %T.", d)
	}

	if typed.bot != bot {
		t.Errorf("Given bot is not set: %+v.", typed.bot)
	}
}

func TestDispatcher_Dispatch(t *testing.T) {
	sent := false
	var destination sarah.OutputDestination
	var content interface{}
	bot := &DummyBot{
		SendMessageFunc: func(_ context.Context, o sarah.Output) {
			sent = true
			destination = o.Destination()
			content = o.Content()
		},
	}
	d := &dispatcher{bot: bot}
	destinationArg := &Destination{}
	contentArg := struct{}{}

	err := d.Dispatch(destinationArg, contentArg)

	if err != nil {
		t.Fatalf("Error occurred: %s.", err.Error())
	}

	if !sent {
		t.Errorf("Bot.SendMessage is not called.")
	}

	if destination != destinationArg {
		t.Errorf("Given destination is not passed: %+v.", destination)
	}

	if content != contentArg {
		t.Errorf("Given content is not passed: %+v.", content)
	}
}

func TestDispatcher_DispatchTransactional(t *testing.T) {
	sent := false
	var destination sarah.OutputDestination
	var content interface{}
	res := &Response{}
	bot := &DummyBot{
		SendMessageFunc: func(_ context.Context, o sarah.Output) {
			sent = true
			destination = o.Destination()
			content = o.Content()
			to := o.(*transactionalOutput)
			to.c <- []*Response{res}
		},
	}
	d := &dispatcher{bot: bot}
	destinationArg := &Destination{}
	contentArg := struct{}{}

	responses, err := d.DispatchTransactional(destinationArg, contentArg, 1*time.Second)

	if err != nil {
		t.Fatalf("Error occurred: %s.", err.Error())
	}

	if !sent {
		t.Errorf("Bot.SendMessage is not called.")
	}

	if destination != destinationArg {
		t.Errorf("Given destination is not passed: %+v.", destination)
	}

	if content != contentArg {
		t.Errorf("Given content is not passed: %+v.", content)
	}

	if len(responses) != 1 {
		t.Fatalf("Unexpected amount of *Response is returned: %d.", len(responses))
	}

	if responses[0] != res {
		t.Errorf("Unexpected response is returned: %+v.", responses[0])
	}
}

func TestDispatcher_DispatchTransactional_Timeout(t *testing.T) {
	bot := &DummyBot{
		SendMessageFunc: func(_ context.Context, o sarah.Output) {},
	}
	d := &dispatcher{bot: bot}

	responses, err := d.DispatchTransactional(&Destination{}, struct{}{}, 1*time.Millisecond)

	if err == nil {
		t.Fatalf("Expected error is not returned.")
	}

	if len(responses) != 0 {
		t.Fatalf("Unexpected amount of *Response is returned: %d.", len(responses))
	}
}
