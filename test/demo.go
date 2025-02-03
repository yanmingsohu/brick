package main

import (
	"time"

	"github.com/yanmingsohu/brick/v2"
)


func main() {
	conf := brick.Config{
		HttpPort:7077, 
		SessionExp:1 * time.Hour, 
		CookieName: "testserver",
	}
	b := brick.NewBrick(conf)
	b.StartHttpServer()
}