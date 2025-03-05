package main

import (
	"flag"
	"mesher/mesher"
)

func main() {
	address := flag.String(
		"address",
		":8981",
		"local address to listen for udp packages for")
	flag.Parse()
	done := mesher.Server(*address)
	<-done
}
