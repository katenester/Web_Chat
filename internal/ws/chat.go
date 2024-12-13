package ws

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/5aradise/go-message/internal/types"
	"github.com/gorilla/websocket"
)

type chat struct {
	users    map[string]*user
	mu       sync.RWMutex
	broadCh  chan types.Message
	stopCh   chan signal
	messages []types.Message
}

func NewChat() *chat {
	c := &chat{
		users:    make(map[string]*user, 10),
		mu:       sync.RWMutex{},
		broadCh:  make(chan types.Message, 10),
		stopCh:   make(chan signal),
		messages: make([]types.Message, 0, 128),
	}
	return c
}

func (c *chat) Run() {
	for {
		select {
		case msg := <-c.broadCh:
			c.broadcast(msg)
		case <-c.stopCh:
			return
		}
	}
}

func (c *chat) broadcast(msg types.Message) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.users[msg.Sender]
	if !ok && msg.Sender != "" {
		return
	}
	c.messages = append(c.messages, msg)
	for _, u := range c.users {
		u.ch <- msg
	}
}

func (c *chat) CreateUser(name string, w http.ResponseWriter, r *http.Request, upgrader websocket.Upgrader) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	u, ok := c.users[name]
	if ok {
		connId, err := u.AddConn(w, r, upgrader)
		if err != nil {
			return err
		}

		go u.ListenConn(connId)

		u.WriteSliceMsgsToConn(connId, c.messages)

		return nil
	}

	u = NewUser(name, c)
	connId, err := u.AddConn(w, r, upgrader)
	if err != nil {
		return err
	}

	go u.ListenChat()
	go u.ListenConn(connId)

	c.users[name] = u

	u.WriteSliceMsgsToConn(connId, c.messages)

	c.sendChatMsg(fmt.Sprintf("%s has joined the chat", u.name), time.Now())
	return nil
}

func (c *chat) DeleteUser(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	u, ok := c.users[name]
	if !ok {
		log.Println("chat: unfound user by name:", name)
		return
	}
	delete(c.users, name)
	for _, conn := range u.conns {
		if conn != nil {
			err := conn.Close()
			if err != nil {
				log.Println("chat: conn.Close:", err)
			}
		}
	}

	c.sendChatMsg(fmt.Sprintf("%s has left the chat", u.name), time.Now())
}

func (c *chat) sendChatMsg(body string, time time.Time) {
	c.broadCh <- types.NewMessage("", body, time)
}
