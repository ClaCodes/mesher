package main

import (
	"log"
	"mesher/mesher"
	"net"
	"time"
)

func client(messages chan mesher.Msg, tx chan []byte) {
	go func() {
		var peers []net.UDPAddr
		helloTimeout := time.NewTicker(3 * time.Second)
		peerTimeout := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-helloTimeout.C:
				var txbuf [1]byte
				txbuf[0] = mesher.ClientHello
				tx <- txbuf[:]
			case <-peerTimeout.C:
				for _, p := range peers {
					var txbuf [20]byte
					txbuf[0] = mesher.ClientRelayTo
					mesher.EncodeAddr(txbuf[1:19], p)
					txbuf[19] = 'b'
					tx <- txbuf[:]
				}
			case msg := <-messages:
				switch msg.Type {
				case mesher.ServerHello:
					log.Println("Received ServerHello from", msg.Src)
					bufs := make([]net.UDPAddr, 0)
					n := len(msg.Buf)
					i := 0
					for n-i >= 18 {
						a := mesher.DecodeAddr(msg.Buf[i : i+18])
						i += 18
						bufs = append(bufs, a)
					}
					peers = bufs
				case mesher.ServerRelayFrom:
					if len(msg.Buf) >= 18 {
						from := mesher.DecodeAddr(msg.Buf[:18])
						log.Println("Received ServerRelayFrom from", from)
						log.Println("Relayed data:", string(msg.Buf[18:]))
					}
				}
			}
		}
	}()
}

func main() {

	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8981")
	if err != nil {
		log.Fatal(err)
	}

	localAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", localAddr, serverAddr)
	if err != nil {
		log.Fatal(err)
	}

	tx := make(chan []byte)
	mesher.Writer(*serverAddr, conn, tx)
	messages := mesher.Reader(conn)
	client(messages, tx)

	for {
	}

	conn.Close()

}
