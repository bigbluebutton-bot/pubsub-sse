package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/apex/log"
)

// SSEPubSubHandler represents the SSE publisher and subscriber system.
type sSEPubSubHandler struct {
	clients      map[string]*client
	publicTopics map[string]*topic
	lock         sync.RWMutex
	Timeout      time.Duration

	ClientIDQueryParameter string
	TopicQueryParameter    string
}

// NewSSEPubSub creates a new sSEPubSubHandler instance.
func NewSSEPubSubHandler() *sSEPubSubHandler {
	return &sSEPubSubHandler{
		clients:      make(map[string]*client),
		publicTopics: make(map[string]*topic),
		Timeout:      10 * time.Second,

		ClientIDQueryParameter: "client_id",
		TopicQueryParameter:    "topic",
	}
}

// AddClient handles HTTP requests for adding a new client.
func (s *sSEPubSubHandler) AddClient(w http.ResponseWriter, r *http.Request) {
    // GET clientID from request body
    clientID := r.URL.Query().Get(s.ClientIDQueryParameter)

    // Add client
    s.NewClient(clientID)

    // return 200 ok with json ok
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"ok": true}`)
}

// Subscribe handles HTTP requests for client subscriptions.
func (s *sSEPubSubHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	// GET clientID and topic from request body
	clientID := r.URL.Query().Get(s.ClientIDQueryParameter)
	topic := r.URL.Query().Get(s.TopicQueryParameter)

	s.lock.Lock()
	defer s.lock.Unlock()

	// Find client
	client, exists := s.clients[clientID]
	if !exists {
		// Send error if client does not exists 404 with json
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"ok": false, "error": "client %s does not exists"}`, clientID)
		return
	}

	// Find topic.
	clTop := client.GetTopics()
	if _, exists := clTop[topic]; !exists {
		// Send error if topic does not exists 404 with json
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"ok": false, "error": "topic %s does not exists"}`, clientID)
		return
	}

	// Add client to topic
	client.Sub(topic)

	// return 200 ok with json ok
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"ok": true}`)
}

// Unsibscribe handles HTTP requests for client unsubscriptions.
func (s *sSEPubSubHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	// GET clientID and topic from request body
	clientID := r.URL.Query().Get(s.ClientIDQueryParameter)
	topic := r.URL.Query().Get(s.TopicQueryParameter)

	s.lock.Lock()
	defer s.lock.Unlock()

	// Find client
	client, exists := s.clients[clientID]
	if !exists {
		// Send error if client does not exists 404 with json
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"ok": false, "error": "client %s does not exists"}`, clientID)
		return
	}

	// Find topic.
	clTop := client.GetTopics()
	if _, exists := clTop[topic]; !exists {
		// Send error if topic does not exists 404 with json
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"ok": false, "error": "topic %s does not exists"}`, clientID)
		return
	}

	// Remove client from topic
	client.Unsub(topic)

	// return 200 ok with json ok
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"ok": true}`)
}

func (s *sSEPubSubHandler) Event(w http.ResponseWriter, r *http.Request) {
	// GET clientID and topic from request body
	clientID := r.URL.Query().Get(s.ClientIDQueryParameter)

	s.lock.Lock()

	// Find client
	client, exists := s.clients[clientID]
	if !exists {
		// Send error if client does not exists
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusNotFound)
        fmt.Fprintf(w, `{"ok": false, "error": "client %s does not exists"}`, clientID)
        s.lock.Unlock()
		return
	}

	s.lock.Unlock()

	// SSE-specific headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Keep the connection open until it's closed by the client
	for {
		select {
		case msg := <-client.stream:
            log.Infof("Sending message to client %s: %s", clientID, msg)
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		// case <-time.After(s.Timeout):
		// 	return
		// }
        default:
            time.Sleep(100 * time.Millisecond)
        }
	}
}

// Publish sends a message to all subscribed clients on a topic.
func (s *sSEPubSubHandler) Pub(topic string, message interface{}) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	// Find topic.
	t, exists := s.publicTopics[topic]
	if !exists {
		return fmt.Errorf("topic %s does not exists", topic)
	}

	for _, client := range t.Clients {
        // Convert message to json
        jsonMessage, err := client.generateUpdateData(t, message)
        if err != nil {
            return err
        }

		client.stream <- string(jsonMessage)
	}

	return nil
}
