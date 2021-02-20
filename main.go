package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.html"))
}

const patience time.Duration = time.Second * 1

var msg = make(chan string)
var broker = NewServer()

type Broker struct {
	Notifier chan []byte

	newClients chan chan []byte

	closingClients chan chan []byte

	clients map[chan []byte]bool
}

func main() {
	go sendMessage(broker)
	http.HandleFunc("/chat", chat)
	http.HandleFunc("/message", getPost)
	http.HandleFunc("/", index)
	http.ListenAndServe("localhost:3000", nil)

}

func index(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "index.html", nil)
}

func chat(w http.ResponseWriter, r *http.Request) {

	flusher, ok := w.(http.Flusher)

	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	messageChan := make(chan []byte)

	broker.newClients <- messageChan

	defer func() {
		broker.closingClients <- messageChan
	}()

	notify := w.(http.CloseNotifier).CloseNotify()

	for {
		select {
		case <-notify:
			return
		default:

			fmt.Fprintf(w, "data: %s\n\n", <-messageChan)

			flusher.Flush()
		}
	}

}

func NewServer() (broker *Broker) {

	broker = &Broker{
		Notifier:       make(chan []byte, 1),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}

	go listen(broker)

	return
}

func listen(broker *Broker) {
	for {
		select {
		case s := <-broker.newClients:

			broker.clients[s] = true
			log.Printf("Client added. %d registered clients", len(broker.clients))
		case s := <-broker.closingClients:

			delete(broker.clients, s)
			log.Printf("Removed client. %d registered clients", len(broker.clients))
		case event := <-broker.Notifier:

			for clientMessageChan, _ := range broker.clients {
				select {
				case clientMessageChan <- event:
				case <-time.After(patience):
					log.Print("Skipping client.")
				}
			}
		}
	}

}
func sendMessage(broker *Broker) {
	for {
		broker.Notifier <- []byte(<-msg)
	}
}
func getMessage(message string) {

	msg <- message

}

func getPost(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)

	getMessage(string(body))
}
