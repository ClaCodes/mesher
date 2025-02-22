package main

import (
	"mesher/mesher"
	"time"
)

func main() {
	address := "127.0.0.1:8981"
	broadcast := mesher.Bonder(address)

	peerTimeout := time.NewTicker(5 * time.Second)
	var sending byte
	sending = 60
	for {
		<-peerTimeout.C
		var txbuf [1]byte
		txbuf[0] = sending
		broadcast <- txbuf[:]
		sending += 1
		if sending > 75 {
			sending = 60
		}
	}
}
