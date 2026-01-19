package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

var DB *sql.DB
var clients = make(map[*websocket.Conn]bool)
var clientsMu = sync.Mutex{}


func getCount(){
  rows, err := DB.Query("SELECT * from visitors")
	if err != nil {
		log.Println("Error reading DB ")
	}
	defer rows.Close()

	for rows.Next(){
		var count int
		var timeStr string

		if err := rows.Scan(&count, &timeStr); err != nil {
			log.Println("Error scanning row", err)
			continue
		}
	  fmt.Printf("Count: %d, Time: %s\n", count, timeStr)	
	}

	fmt.Println("Closing DB")
	DB.Close()
}

func initDB(){
	var err error
	DB, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}

	sqlStmt := `
		CREATE TABLE IF NOT EXISTS visitors (
			count INTEGER,
			time TEXT
		)
	`
	_, err = DB.Exec(sqlStmt)
	if err != nil {
		log.Fatalf("Error creating tables: %q",err)
	}
}

func insertIntoDB(numOfClients int){
  stmt, err := DB.Prepare("INSERT INTO VISITORS (count, time) VALUES (?, datetime('now'))")
	if err != nil {
		log.Fatalln("Could not run prepare statement")
	}
	stmt.Exec(numOfClients)
}


var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func broadcast(c *websocket.Conn) int {
	clientsMu.Lock()
	count := len(clients)
	clientsMu.Unlock()

	c.WriteMessage(
		websocket.TextMessage,
		[]byte(fmt.Sprintf("Current Visitors: %d", count)),
	)
	return count
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error while upgrading", err)
			return
		}

		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()

		defer func() {
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			conn.Close()
		}()

		if err := conn.WriteMessage(websocket.TextMessage, []byte("Welcome to my server")); err != nil {
			fmt.Println("Error sending Message", err)
			return
		}
		
		numOfClients := broadcast(conn)
		insertIntoDB(numOfClients)

		for {
			_, _, err := conn.ReadMessage()

			if err != nil {
				fmt.Println("Error reading message", err)
				break
			}
		}
		return
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./index.html")
}

func main() {
	initDB()
	defer DB.Close()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func(){
		sig := <-sigs
		fmt.Println("Received signal:", sig)
	  getCount()	
		os.Exit(0)
	}()

	http.HandleFunc("/", mainHandler)
	http.HandleFunc("/ws", wsHandler)

	fmt.Println("Websocket server started on :3000")
	err := http.ListenAndServe(":3000", nil)
	if err != nil {
		fmt.Println("Error starting server", err)
	}
}
