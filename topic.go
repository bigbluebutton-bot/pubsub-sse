package pubsubsse

import (
	"sync"

	"github.com/apex/log"
	"github.com/google/uuid"
)

// Topic Types
type topicType string

const (
	TPublic  topicType = "public"
	TPrivate topicType = "private"
	TGroup   topicType = "group"
)

// Topic represents a messaging Topic in the SSE pub-sub system.
type Topic struct {
	name    string
	id      string
	ttype   topicType
	clients map[string]*Client
	lock    sync.Mutex
}

// Create a new topic
func newTopic(name string, ttype topicType) *Topic {
	return &Topic{
		name:    name,
		id:      uuid.New().String(),
		ttype:   ttype,
		clients: make(map[string]*Client),
	}
}

// Get Name
func (t *Topic) GetName() string {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.name
}

// Get ID
func (t *Topic) GetID() string {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.id
}

// Get Type
func (t *Topic) GetType() string {
	t.lock.Lock()
	defer t.lock.Unlock()

	return string(t.ttype)
}

// Add a client to the topic
func (t *Topic) addClient(c *Client) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.clients[c.id] = c
}

// Remove a client from the topic
func (t *Topic) removeClient(c *Client) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.clients, c.id)
}

// Get all clients in the topic
func (t *Topic) GetClients() map[string]*Client {
	t.lock.Lock()
	defer t.lock.Unlock()

	// Create a copy of the map
	newmap := make(map[string]*Client)
	for k, v := range t.clients {
		newmap[k] = v
	}
	return newmap
}

// Check if a client is subscribed to the topic
func (t *Topic) IsSubscribed(c *Client) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	_, ok := t.clients[c.id]
	return ok
}

type eventData struct {
	Sys     []eventDataSys     `json:"sys"`
	Updates []eventDataUpdates `json:"updates"`
}

type eventDataSys struct {
	Type string             `json:"type"`
	List []eventDataSysList `json:"list,omitempty"`
}

type eventDataSysList struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // topics, subscribed, unsubscribed
}

type eventDataUpdates struct {
	Topic string      `json:"topic"`
	Data  interface{} `json:"data"`
}

// Publish a message to all clients in the topic
func (t *Topic) Pub(msg interface{}) error {
	// Build the JSON data
	fulldata := &eventData{
		Updates: []eventDataUpdates{},
	}
	u := eventDataUpdates{
		Topic: t.GetName(),
		Data:  msg,
	}
	fulldata.Updates = append(fulldata.Updates, u)

	// Send the JSON data to all clients
	for _, c := range t.GetClients() {
		err := c.send(fulldata) // ignore error. Fire and forget.
		if err != nil {
			log.Errorf("[T:%s]: Error sending data to client: %s", t.GetName(), err.Error())
		}
	}

	return nil
}
