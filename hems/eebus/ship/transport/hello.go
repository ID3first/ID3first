package transport

import (
	"errors"
	"fmt"
	"time"

	"github.com/andig/evcc/hems/eebus/ship/message"
)

// Hello is the common hello exchange
func (c *Transport) Hello() error {
	// SME_HELLO_STATE_READY_INIT
	if err := c.WriteJSON(message.CmiTypeControl, message.CmiConnectionHello{
		ConnectionHello: message.ConnectionHello{
			Phase: string(message.ConnectionHelloPhaseTypeReady),
		},
	}); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	timer := time.NewTimer(message.CmiTimeout)
	for {
		// SME_HELLO_STATE_READY_LISTEN
		msg, err := c.ReadMessage(timer.C)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				// SME_HELLO_STATE_READY_TIMEOUT
				_ = c.WriteJSON(message.CmiTypeControl, message.CmiConnectionHello{
					ConnectionHello: message.ConnectionHello{
						Phase: string(message.ConnectionHelloPhaseTypeAborted),
					},
				})
			}

			return err
		}

		switch hello := msg.(type) {
		case message.ConnectionHello:
			switch hello.Phase {
			case string(message.ConnectionHelloPhaseTypeReady):
				// HELLO_OK
				return nil

			case string(message.ConnectionHelloPhaseTypeAborted):
				return errors.New("hello: aborted")

			case string(message.ConnectionHelloPhaseTypePending):
				if hello.ProlongationRequest != nil && *hello.ProlongationRequest {
					timer = time.NewTimer(message.CmiHelloProlongationTimeout)
				}
			}

		case message.ConnectionClose:
			err = errors.New("hello: remote closed")

		default:
			return errors.New("hello: invalid type")
		}
	}
}