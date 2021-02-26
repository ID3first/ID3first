package ship

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/andig/evcc/hems/eebus/ship/message"
	"github.com/andig/evcc/hems/eebus/ship/transport"
	"github.com/andig/evcc/hems/eebus/util"
	"github.com/gorilla/websocket"
)

// Client is the ship client
type Client struct {
	mux    sync.Mutex
	Log    util.Logger
	Local  Service
	Remote Service
	t      *transport.Transport
	closed bool
}

// init creates the connection
func (c *Client) init() error {
	init := []byte{message.CmiTypeInit, 0x00}

	// CMI_STATE_CLIENT_SEND
	if err := c.t.WriteBinary(init); err != nil {
		return err
	}

	timer := time.NewTimer(message.CmiTimeout)

	// CMI_STATE_CLIENT_WAIT
	msg, err := c.t.ReadBinary(timer.C)
	if err != nil {
		return err
	}

	// CMI_STATE_CLIENT_EVALUATE
	if bytes.Compare(init, msg) != 0 {
		return fmt.Errorf("init: invalid response")
	}

	return nil
}

func (c *Client) protocolHandshake() error {
	hs := message.CmiHandshakeMsg{
		MessageProtocolHandshake: message.MessageProtocolHandshake{
			HandshakeType: message.ProtocolHandshakeTypeAnnounceMax,
			Version:       message.Version{Major: 1, Minor: 0},
			Formats:       message.Format{Format: []string{message.ProtocolHandshakeFormatJSON}},
		},
	}
	if err := c.t.WriteJSON(message.CmiTypeControl, hs); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	// receive server selection and send selection back to server
	err := c.t.HandshakeReceiveSelect()
	if err == nil {
		hs.MessageProtocolHandshake.HandshakeType = message.ProtocolHandshakeTypeSelect
		err = c.t.WriteJSON(message.CmiTypeControl, hs)
	}

	return err
}

// Close performs ordered close of client connection
func (c *Client) Close() error {
	c.mux.Lock()
	defer c.mux.Unlock()

	if c.closed {
		return os.ErrClosed
	}

	c.closed = true

	// stop readPump
	// defer close(c.closeC)

	return c.t.Close()
}

// Connect performs the client connection handshake
func (c *Client) Connect(conn *websocket.Conn) error {
	c.t = transport.New(c.Log, conn)

	if err := c.init(); err != nil {
		return err
	}

	err := c.t.Hello()
	if err == nil {
		err = c.protocolHandshake()
	}
	if err == nil {
		err = c.t.PinState(c.Local.Pin, c.Remote.Pin)
	}
	if err == nil {
		c.Remote.Methods, err = c.t.AccessMethodsRequest(c.Local.Methods)
	}

	// close connection if handshake or hello fails
	if err != nil {
		_ = c.t.Close()
	}

	return err
}