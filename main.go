package main

import (
	"fmt"
	"github.com/Shopify/sarama"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ConsumerConfig struct {
	Broker    string `json:"broker"`
	Partition int    `json:"partition"`
	Topic     string `json:"topic"`
	Offset    string `json:"offset"`
}

type Config struct {
	Consumers []ConsumerConfig `json:"consumers"`
}

type Link struct {
	Url   string
	Title string
}

func startConsumers(config *Config, c chan *sarama.ConsumerMessage, quit chan bool) {
	for _, consumerConfig := range config.Consumers {
		topic, broker, partition := consumerConfig.Topic, consumerConfig.Broker, consumerConfig.Partition
		var offset int64 = -1

		if numericOffset, err := strconv.ParseInt(consumerConfig.Offset, 10, 64); err == nil {
			offset = numericOffset
		} else {
			switch consumerConfig.Offset {
			case "oldest":
				offset = -2
			case "newest":
				offset = -1
			default:
				log.Println("Invalid value for consumer offset")
				quit <- true
				return
			}
		}

		go consume(c, quit, topic, broker, partition, offset)
	}
}

func onConnected(ws *websocket.Conn) {
	log.Println("Opened WebSocket connection!")

	var config Config
	err := websocket.JSON.Receive(ws, &config)
	if err != nil {
		ws.Close()
		log.Println("Didn't receive config from WebSocket!", err)
		return
	}

	quit := make(chan bool)
	c := make(chan *sarama.ConsumerMessage)

	startConsumers(&config, c, quit)

	for {
		select {
		case consumerMessage := <-c:
			msg :=
				"{\"topic\": \"" + consumerMessage.Topic +
					"\", \"partition\": \"" + strconv.FormatInt(int64(consumerMessage.Partition), 10) +
					"\", \"offset\": \"" + strconv.FormatInt(consumerMessage.Offset, 10) +
					"\", \"key\": \"" + strings.Replace(string(consumerMessage.Key), `"`, `\"`, -1) +
					"\", \"value\": \"" + strings.Replace(string(consumerMessage.Value), `"`, `\"`, -1) +
					"\", \"consumedUnixTimestamp\": \"" + strconv.FormatInt(time.Now().Unix(), 10) +
					"\"}\n"

			log.Println("Sending message to WebSocket: " + msg)
			err := websocket.Message.Send(ws, msg)
			if err != nil {
				log.Println("Error while trying to send to WebSocket: ", err)
				quit <- true
			}
		case <-quit:
			err := ws.Close()
			log.Println("Closed WebSocket connection given quit signal.")
			if err != nil {
				log.Println("Error while closing WebSocket!: ", err)
			}
			return
		}
	}
}

func baseHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && r.URL.RawQuery == "" {
		serveBaseHTML(w, r)
	} else {
		log.Println(r.URL.Path)
		http.FileServer(http.Dir("webroot")).ServeHTTP(w, r)
	}
}

func listenToSignals() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		os.Exit(1)
	}()
}

var mux = http.NewServeMux()

func main() {
	listenToSignals()

	mux.Handle("/ws", websocket.Handler(onConnected))
	mux.HandleFunc("/", baseHandler)

	port := "41234"
	if len(os.Args) >= 2 {
		port = os.Args[1]
	}

	fmt.Printf("Flowbro is your bro on localhost:%v!\n", port)
	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
