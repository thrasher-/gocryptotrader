package stream

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// WebsocketConnection contains all the data needed to send a message to a WS
// connection
type WebsocketConnection struct {
	Verbose   bool
	connected int32

	// Gorilla websocket does not allow more than one goroutine to utilise
	// writes methods
	writeControl sync.Mutex

	RateLimit    float64
	ExchangeName string
	URL          string
	ProxyURL     string
	Wg           *sync.WaitGroup
	Connection   *websocket.Conn
	ShutdownC    chan struct{}

	Match             *Match
	ResponseMaxLimit  time.Duration
	Traffic           chan struct{}
	readMessageErrors chan error
}

// SendMessageReturnResponse will send a WS message to the connection and wait
// for response
func (w *WebsocketConnection) SendMessageReturnResponse(signature, request interface{}) ([]byte, error) {
	m, err := w.Match.set(signature)
	if err != nil {
		return nil, err
	}
	defer m.Cleanup()

	err = w.SendJSONMessage(request)
	if err != nil {
		return nil, err
	}

	timer := time.NewTimer(w.ResponseMaxLimit)

	select {
	case payload := <-m.C:
		return payload, nil
	case <-timer.C:
		timer.Stop()
		return nil, fmt.Errorf("timeout waiting for response with signature: %v", signature)
	}
}

// Dial sets proxy urls and then connects to the websocket
func (w *WebsocketConnection) Dial(dialer *websocket.Dialer, headers http.Header) error {
	if w.ProxyURL != "" {
		proxy, err := url.Parse(w.ProxyURL)
		if err != nil {
			return err
		}
		dialer.Proxy = http.ProxyURL(proxy)
	}

	var err error
	var conStatus *http.Response

	w.Connection, conStatus, err = dialer.Dial(w.URL, headers)
	if err != nil {
		if conStatus != nil {
			return fmt.Errorf("%v %v %v Error: %v",
				w.URL,
				conStatus,
				conStatus.StatusCode,
				err)
		}
		return fmt.Errorf("%v Error: %v", w.URL, err)
	}
	if w.Verbose {
		log.Infof(log.WebsocketMgr,
			"%v Websocket connected to %s",
			w.ExchangeName,
			w.URL)
	}
	w.Traffic <- struct{}{}
	w.setConnectedStatus(true)
	return nil
}

// SendJSONMessage sends a JSON encoded message over the connection
func (w *WebsocketConnection) SendJSONMessage(data interface{}) error {
	if !w.IsConnected() {
		return fmt.Errorf("%v cannot send message to a disconnected websocket",
			w.ExchangeName)
	}

	w.writeControl.Lock()
	defer w.writeControl.Unlock()

	if w.Verbose {
		log.Debugf(log.WebsocketMgr,
			"%v sending message to websocket %+v", w.ExchangeName, data)
	}

	if w.RateLimit > 0 {
		time.Sleep(time.Duration(w.RateLimit) * time.Millisecond)
		if !w.IsConnected() {
			return fmt.Errorf("%v cannot send message to a disconnected websocket",
				w.ExchangeName)
		}
	}
	return w.Connection.WriteJSON(data)
}

// SendRawMessage sends a message over the connection without JSON encoding it
func (w *WebsocketConnection) SendRawMessage(messageType int, message []byte) error {
	if !w.IsConnected() {
		return fmt.Errorf("%v cannot send message to a disconnected websocket",
			w.ExchangeName)
	}

	w.writeControl.Lock()
	defer w.writeControl.Unlock()

	if w.Verbose {
		log.Debugf(log.WebsocketMgr,
			"%v sending message to websocket %s",
			w.ExchangeName,
			message)
	}
	if w.RateLimit > 0 {
		time.Sleep(time.Duration(w.RateLimit) * time.Millisecond)
		if !w.IsConnected() {
			return fmt.Errorf("%v cannot send message to a disconnected websocket",
				w.ExchangeName)
		}
	}
	return w.Connection.WriteMessage(messageType, message)
}

// SetupPingHandler will automatically send ping or pong messages based on
// WebsocketPingHandler configuration
func (w *WebsocketConnection) SetupPingHandler(handler PingHandler) {
	if handler.UseGorillaHandler {
		h := func(msg string) error {
			err := w.Connection.WriteControl(handler.MessageType,
				[]byte(msg),
				time.Now().Add(handler.Delay))
			if err == websocket.ErrCloseSent {
				return nil
			} else if e, ok := err.(net.Error); ok && e.Temporary() {
				return nil
			}
			return err
		}
		w.Connection.SetPingHandler(h)
		return
	}
	w.Wg.Add(1)
	defer w.Wg.Done()
	go func() {
		ticker := time.NewTicker(handler.Delay)
		for {
			select {
			case <-w.ShutdownC:
				ticker.Stop()
				return
			case <-ticker.C:
				err := w.SendRawMessage(handler.MessageType, handler.Message)
				if err != nil {
					log.Errorf(log.WebsocketMgr,
						"%v failed to send message to websocket %s",
						w.ExchangeName,
						handler.Message)
					return
				}
			}
		}
	}()
}

func (w *WebsocketConnection) setConnectedStatus(b bool) {
	if b {
		atomic.StoreInt32(&w.connected, 1)
		return
	}
	atomic.StoreInt32(&w.connected, 0)
}

// IsConnected exposes websocket connection status
func (w *WebsocketConnection) IsConnected() bool {
	return atomic.LoadInt32(&w.connected) == 1
}

// ReadMessage reads messages, can handle text, gzip and binary
func (w *WebsocketConnection) ReadMessage() (Response, error) {
	mType, resp, err := w.Connection.ReadMessage()
	if err != nil {
		if isDisconnectionError(err) {
			w.setConnectedStatus(false)
			w.readMessageErrors <- err
		}
		return Response{}, err
	}

	select {
	case w.Traffic <- struct{}{}:
	default: // causes contention, just bypass if there is no receiver.
	}

	var standardMessage []byte
	switch mType {
	case websocket.TextMessage:
		standardMessage = resp
	case websocket.BinaryMessage:
		standardMessage, err = w.parseBinaryResponse(resp)
		if err != nil {
			return Response{}, err
		}
	}
	if w.Verbose {
		log.Debugf(log.WebsocketMgr,
			"%v Websocket message received: %v",
			w.ExchangeName,
			string(standardMessage))
	}
	return Response{Raw: standardMessage, Type: mType}, nil
}

// parseBinaryResponse parses a websocket binary response into a usable byte array
func (w *WebsocketConnection) parseBinaryResponse(resp []byte) ([]byte, error) {
	var standardMessage []byte
	var err error
	// Detect GZIP
	if resp[0] == 31 && resp[1] == 139 {
		b := bytes.NewReader(resp)
		var gReader *gzip.Reader
		gReader, err = gzip.NewReader(b)
		if err != nil {
			return standardMessage, err
		}
		standardMessage, err = ioutil.ReadAll(gReader)
		if err != nil {
			return standardMessage, err
		}
		err = gReader.Close()
		if err != nil {
			return standardMessage, err
		}
	} else {
		reader := flate.NewReader(bytes.NewReader(resp))
		standardMessage, err = ioutil.ReadAll(reader)
		if err != nil {
			return standardMessage, err
		}
		err = reader.Close()
		if err != nil {
			return standardMessage, err
		}
	}
	return standardMessage, nil
}

// GenerateMessageID Creates a messageID to checkout
func (w *WebsocketConnection) GenerateMessageID(useNano bool) int64 {
	if useNano {
		// force clock shift
		time.Sleep(time.Nanosecond)
		return time.Now().UnixNano()
	}
	return time.Now().Unix()
}

// Shutdown shuts down and closes specific connection
func (w *WebsocketConnection) Shutdown() error {
	if w == nil || w.Connection == nil {
		return nil
	}
	return w.Connection.UnderlyingConn().Close()
}

// SetURL sets connection URL
func (w *WebsocketConnection) SetURL(url string) {
	w.URL = url
	return
}

// SetProxy sets connection proxy
func (w *WebsocketConnection) SetProxy(proxy string) {
	w.ProxyURL = proxy
	return
}