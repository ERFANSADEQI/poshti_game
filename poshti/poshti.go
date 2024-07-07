package poshti

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type message []interface{}

type Message struct {
	MessageRef int
	JoinRef    int
	ChannelAdr string
	Topic      string
	Payload    interface{}
}

type Callback = func(ch, topic string, payload interface{})

type Client struct {
	url          string
	pid          string
	conn         *websocket.Conn
	callbacks    map[string]Callback
	lastActivity time.Time
	mu           sync.Mutex
}

func (msg *Message) messageToRawMessage() message {
	return message{
		fmt.Sprint(msg.MessageRef),
		fmt.Sprint(msg.JoinRef),
		msg.ChannelAdr,
		msg.Topic,
		msg.Payload,
	}
}

func (msg message) rawMessageToMessage() Message {
	fmt.Println(msg)
	if msg[0] == nil {
		msg[0] = "0"
	}
	if msg[1] == nil {
		msg[1] = "0"
	}
	mref, _ := strconv.Atoi(msg[0].(string))
	jref, _ := strconv.Atoi(msg[1].(string))
	return Message{
		MessageRef: mref,
		JoinRef:    jref,
		ChannelAdr: msg[2].(string),
		Topic:      msg[3].(string),
		Payload:    msg[4],
	}
}

func NewClient(poshti_id string) *Client {
	return &Client{
		url:       "wss://app.poshti.live/socket/websocket?vsn=2.0.0",
		pid:       poshti_id,
		callbacks: make(map[string]Callback),
	}
}

func (c *Client) Connect(token string) error {
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("%s&auth=%s", c.url, token), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	c.conn = conn
	go c.listen()
	go c.ping()
	return nil
}

func (c *Client) listen() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("read error: %v", err)
			return
		}
		c.lastActivity = time.Now()
		var raw message
		if err := json.Unmarshal(msg, &raw); err != nil {
			log.Printf("unmarshal error: %v", err)
			continue
		}
		message := raw.rawMessageToMessage()
		c.handleMessage(message)
	}
}

func (c *Client) handleMessage(msg Message) {
	// c.mu.Lock()
	// defer c.mu.Unlock()
	ch_name := extractChannelName(msg.ChannelAdr)
	if callback, ok := c.callbacks[ch_name]; ok {
		callback(ch_name, msg.Topic, msg.Payload)
	} else {
		log.Printf("No callback found for channel %s", ch_name)
	}
	// if _, ok := c.chans[msg.Channel]; ok {
	// 	fmt.Printf("Received message on topic %s: %s\n", msg.Channel, string(msg.Payload))
	// } else {
	// 	log.Printf("No channel found for topic %s", msg.Channel)
	// }
}

func (c *Client) JoinChannel(name string, callback Callback) error {
	c.callbacks[name] = callback

	joinMessage := Message{
		MessageRef: 0,
		JoinRef:    0,
		ChannelAdr: c.constructChannelName(name),
		Topic:      "phx_join",
		Payload:    []byte(""),
	}
	if err := c.sendMessage(joinMessage); err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	return nil
}

func (c *Client) sendMessage(msg Message) error {
	// c.mu.Lock()
	// defer c.mu.Unlock()
	data, err := json.Marshal(msg.messageToRawMessage())
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func (c *Client) Send(ch_name, topic string, payload interface{}) error {
	msg := Message{
		MessageRef: 0,
		JoinRef:    0,
		ChannelAdr: c.constructChannelName(ch_name),
		Topic:      fmt.Sprintf("broadcast:%s", topic),
		Payload:    payload,
	}
	return c.sendMessage(msg)
}

func (c *Client) Leave(ch_name string) error {
	if _, ok := c.callbacks[ch_name]; ok {
		leave_msg := Message{
			MessageRef: 0,
			JoinRef:    0,
			ChannelAdr: c.constructChannelName(ch_name),
			Topic:      "phx_leave",
			Payload:    []byte(""),
		}
		c.sendMessage(leave_msg)
		return nil
	} else {
		return fmt.Errorf("No channel with name %s", ch_name)
	}
}

func (c *Client) ping() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			if time.Since(c.lastActivity) >= 5*time.Second {
				heartbeat := Message{
					MessageRef: 0,
					JoinRef:    0,
					ChannelAdr: "poshti",
					Topic:      "heartbeat",
					Payload:    []byte(""),
				}
				if err := c.sendMessage(heartbeat); err != nil {
					log.Printf("Ping Server Error: %s", err)
					c.mu.Unlock()
					return
				}
				c.mu.Unlock()
			}
		}
	}
}

func (c *Client) constructChannelName(ch_name string) string {
	return fmt.Sprintf("poshti:%s:%s", c.pid, ch_name)
}

func extractChannelName(name string) string {
	return strings.Join(strings.Split(name, ":")[2:], ":")
}
