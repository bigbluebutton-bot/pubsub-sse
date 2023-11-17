package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type status int
const (
	Waiting status = iota
	Receving
)

// Client represents a subscriber with a channel to send messages.
type client struct {
	id     string
	stream chan string
	status status

	lock sync.Mutex

	publicTopics  map[string]*topic
	privateTopics map[string]*topic
}

// Adds a new client to the system.
func (s *sSEPubSubHandler) NewClient(id string) *client {
	s.lock.Lock()
	defer s.lock.Unlock()

	if id == "" {
		id = uuid.New().String()
	}

	// Check if client id already exists
	if _, exists := s.clients[id]; exists {
		return s.clients[id]
	}

	cl := &client{
		id:           id,
		stream:       make(chan string),
		status:       Waiting,

		lock: sync.Mutex{},

		publicTopics: s.publicTopics,
		privateTopics: make(map[string]*topic),
	}

	s.clients[id] = cl

	return cl
}

// Get all clients
func (s *sSEPubSubHandler) GetClients() map[string]*client {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.clients
}

// Get client by id
func (s *sSEPubSubHandler) getClient(id string) (*client, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, exists := s.clients[id]; !exists {
		return nil, fmt.Errorf("client %s does not exists", id)
	}
	return s.clients[id], nil
}

// RemoveClient removes a client from the system.
func (s *sSEPubSubHandler) RemoveClient(id string) error {
	// Get client
	cl, err := s.getClient(id)
	if err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// remove client from all private topics
	for _, topic := range cl.privateTopics {
		cl.Unsub(topic.Name)
	}

	// remove client from all public topics
	for _, topic := range cl.publicTopics {
		cl.Unsub(topic.Name)
	}

	delete(s.clients, id)

	return nil
}

// Add new private topic
func (c *client) NewPrivateTopic(name string) error {
	// if topic already exists, return error
	if _, exists := c.privateTopics[name]; exists {
		return fmt.Errorf("topic %s already exists", name)
	}

	top := &topic{
		Name:    name,
		Type:    Private,
		Clients: make(map[string]*client),
		lock:    sync.Mutex{},
	}

	// // Add this client to the topic (subscribe)
	// top.Clients[c.id] = c

	// Add to list of topics
	c.lock.Lock()
	c.privateTopics[name] = top
	c.lock.Unlock()

	// Send new topics to client
	c.sendNewTopicsList()

	return nil
}

// Remove private topic
func (c *client) RemovePrivateTopic(name string) error {
	// if topic does not exists, return error
	if _, exists := c.privateTopics[name]; !exists {
		return fmt.Errorf("topic %s does not exists", name)
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	// Unsubscribe all clients from this topic
	c.Unsub(name)

	// Remove from list of topics
	delete(c.privateTopics, name)

	return nil
}

// Get all topics of a client
func (c *client) GetTopics() map[string]*topic {
	c.lock.Lock()
	defer c.lock.Unlock()

	topics := make(map[string]*topic)
	for _, topic := range c.publicTopics {
		topics[topic.Name] = topic
	}
	for _, topic := range c.privateTopics {
		topics[topic.Name] = topic
	}

	return topics
}

// Get all subscribed topics of a client
func (c *client) GetSubscribedTopics() map[string]*topic {
	topics := c.GetTopics()
	c.lock.Lock()
	defer c.lock.Unlock()

	subs := make(map[string]*topic)
	for _, topic := range topics {
		if _, exists := topic.Clients[c.id]; exists {
			subs[topic.Name] = topic
		}
	}
	return subs
}

// Subscribe to topic
func (c *client) Sub(name string) error {
	c.lock.Lock()

	// if topic does not exists, return error
	var topic *topic = nil
	// First search in private topics
	// Second search in groups topics
	// Third search in publicprivate topics
	if _, exists := c.privateTopics[name]; exists {
		topic = c.privateTopics[name]
	} else if _, exists := c.publicTopics[name]; exists {
		topic = c.publicTopics[name]
	} else {
		c.lock.Unlock()
		return fmt.Errorf("topic %s does not exists", name)
	}

	// Add this client to the topic
	topic.lock.Lock()
	topic.Clients[c.id] = c
	topic.lock.Unlock()

	c.lock.Unlock()

	// Send new subscribed topics to client
	c.sendNewSubscribedTopic(topic)

	return nil
}

// Unsubscribe from topic
func (c *client) Unsub(name string) error {
	c.lock.Lock()

	// if topic does not exists, return error
	var topic *topic = nil
	// First search in private topics
	// Second search in groups topics
	// Third search in public topics
	if _, exists := c.privateTopics[name]; exists {
		topic = c.privateTopics[name]
	} else if _, exists := c.publicTopics[name]; exists {
		topic = c.publicTopics[name]
	} else {
		c.lock.Unlock()
		return fmt.Errorf("topic %s does not exists", name)
	}

	// if client is not subscribed to topic, return error
	if _, exists := topic.Clients[c.id]; !exists {
		c.lock.Unlock()
		return fmt.Errorf("client %s is not subscribed to topic %s", c.id, name)
	}

	// Remove this client from the topic
	delete(topic.Clients, c.id)

	c.lock.Unlock()

	// Inform client about unsubscribed topic
	c.sendUnsubscribedTopic(topic)

	return nil
}

// Publish a message
func (c *client) Pub(to string, message interface{}) error {
	if c.status == Waiting {
		return fmt.Errorf("client %s is not receving data", c.id)
	}

	// if topic does not exists, return error
	c.lock.Lock()
	t, exists := c.privateTopics[to]
	if !exists {
		c.lock.Unlock()
		return fmt.Errorf("topic %s does not exists", to)
	}
	c.lock.Unlock()

	// Convert message to json
	err := c.sendUpdate(t, message)
	if err != nil {
		return err
	}

	return nil
}

type eventData struct {
	Sys []eventDataSys `json:"sys"`
	Updates []eventDataUpdates `json:"updates"`
}

type eventDataSys struct {
	Type string `json:"type"`
	List []eventDataSysList `json:"list,omitempty"`
}

type eventDataSysList struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`	// topics, subscribed, unsubscribed
}

type eventDataUpdates struct {
	Topic string      `json:"topic"`
	Data  interface{} `json:"data"`
}

func (c *client) sendUpdate(to *topic, data interface{}) error {
	fulldata := &eventData{
		Updates: []eventDataUpdates{},
	}

	// Updates
	u := eventDataUpdates{
		Topic: to.Name,
		Data:  data,
	}
	fulldata.Updates = append(fulldata.Updates, u)

	jsonData, err := json.Marshal(fulldata)
	if err != nil {
		return err
	}

	c.lock.Lock()
	if c.status == Receving {
		c.stream <- string(jsonData)
	}
	c.lock.Unlock()

	return nil
}

func (c *client) generateInit() (string, error) {
	    // Get all topics of a client
		topics := c.GetTopics()
		subtopics := c.GetSubscribedTopics()
	
		fulldata := &eventData{
			Sys:     []eventDataSys{},
		}
		if len(topics) > 0 {
			fulldata.Sys = append(fulldata.Sys, eventDataSys{})
		}
		if len(subtopics) > 0 {
			fulldata.Sys = append(fulldata.Sys, eventDataSys{})
		}
		// Add all topics and subscribed topics to fulldata
		for _, topic := range topics {
			// Topics
			t := eventDataSysList{
				Name: topic.Name,
				Type: string(topic.Type),
			}
			fulldata.Sys[0].Type = "topics"
			fulldata.Sys[0].List = append(fulldata.Sys[0].List, t)
		}
	
		// Add all subscribed topics to fulldata
		for _, topic := range subtopics {
			t := eventDataSysList{
				Name: topic.Name,
			}
	
			// Subscribed
			fulldata.Sys[1].Type = "subscribed"
			fulldata.Sys[1].List = append(fulldata.Sys[1].List, t)
		}

		jsonData, err := json.Marshal(fulldata)
		if err != nil {
			return "", err
		}

		return string(jsonData), nil
}

func (c *client) sendNewTopicsList() error {
	topics := c.GetTopics()

	fulldata := &eventData{
		Sys:     []eventDataSys{},
	}
	if len(topics) > 0 {
		fulldata.Sys = append(fulldata.Sys, eventDataSys{})

		// Add all topics and subscribed topics to fulldata
		for _, topic := range topics {
			// Topics
			t := eventDataSysList{
				Name: topic.Name,
				Type: string(topic.Type),
			}
			fulldata.Sys[0].Type = "topics"
			fulldata.Sys[0].List = append(fulldata.Sys[0].List, t)
		}
	}

	jsonData, err := json.Marshal(fulldata)
	if err != nil {
		return err
	}

	c.lock.Lock()
	if c.status == Receving {
		c.stream <- string(jsonData)
	}
	c.lock.Unlock()

	return nil
}

func (c *client) sendNewSubscribedTopic(top *topic) error {
	fulldata := &eventData{
		Sys:     []eventDataSys{},
	}
	fulldata.Sys = append(fulldata.Sys, eventDataSys{})

	// Subscribed
	t := eventDataSysList{
		Name: top.Name,
	}
	fulldata.Sys[0].Type = "subscribed"
	fulldata.Sys[0].List = append(fulldata.Sys[0].List, t)

	jsonData, err := json.Marshal(fulldata)
	if err != nil {
		return err
	}

	c.lock.Lock()
	if c.status == Receving {
		c.stream <- string(jsonData)
	}
	c.lock.Unlock()

	return nil
}

func (c *client)sendUnsubscribedTopic(top *topic) error {
	fulldata := &eventData{
		Sys:     []eventDataSys{},
	}
	fulldata.Sys = append(fulldata.Sys, eventDataSys{})

	// Subscribed
	t := eventDataSysList{
		Name: top.Name,
	}
	fulldata.Sys[0].Type = "unsubscribed"
	fulldata.Sys[0].List = append(fulldata.Sys[0].List, t)

	jsonData, err := json.Marshal(fulldata)
	if err != nil {
		return err
	}

	c.lock.Lock()
	if c.status == Receving {
		c.stream <- string(jsonData)
	}
	c.lock.Unlock()

	return nil
}