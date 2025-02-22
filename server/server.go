package main

import (
	"log"
	"mesher/mesher"
	"net"
	"time"
)

func watchdog(addr net.UDPAddr, timeout chan net.UDPAddr) chan struct{} {
	channel := make(chan struct{})
	go func() {
		for {
			select {
			case <-channel:
			case <-time.After(5 * time.Second):
				timeout <- addr
				return
			}
		}
	}()
	return channel
}

type client struct {
	watchdog chan struct{}
	tx       chan []byte
}

func serve(port string) {
    localAddr, err := net.ResolveUDPAddr("udp", port)
    if err != nil {
        log.Fatal(err)
    }
    conn, err := net.ListenUDP("udp", localAddr)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    messages := mesher.Reader(conn)

    timeout := make(chan net.UDPAddr)
    clients := make(map[mesher.Address]client)
    ticker := time.NewTicker(1 * time.Second)
    for {
        select {
        case <-ticker.C:
            for addr, c := range clients {
                buf := make([]byte, 0)
                buf = append(buf, mesher.ServerHello)
                for k, _ := range clients {
                    if k != addr {
                        buf = append(buf, k[:]...)
                    }
                }
                log.Println("Sent     ServerHello to  ", mesher.DecodeAddr(addr[:]), ":", buf[1:])
                c.tx <- buf
            }
        case a := <-timeout:
            var m mesher.Address
            mesher.EncodeAddr(m[:], a)
            c, ok := clients[m]
            if ok {
                close(c.tx)
                delete(clients, m)
            }
            log.Println("Timed out ", a)
        case msg := <-messages:
            switch msg.Type {
            case mesher.ClientHello:
                log.Println("Received ClientHello from", msg.Src)
                var m mesher.Address
                mesher.EncodeAddr(m[:], msg.Src)
                c, ok := clients[m]
                if !ok {
                    tx := make(chan []byte)
                    mesher.WriterTo(msg.Src, conn, tx)
                    c = client{
                        watchdog(msg.Src, timeout),
                        tx,
                    }
                    clients[m] = c
                }
                c.watchdog <- struct{}{}
            case mesher.ClientRelayTo:
                log.Println("Received ClientRelayTo from", msg.Src)
                if len(msg.Buf) >= 18 {
                    var m mesher.Address
                    dst := mesher.DecodeAddr(msg.Buf[:18])
                    mesher.EncodeAddr(m[:], dst)
                    c, ok := clients[m]
                    if !ok {
                        log.Println("Relaying from", msg.Src, "to", dst, "but client not present")
                    } else {
                        buf := make([]byte, 65536)
                        buf[0] = mesher.ServerRelayFrom
                        mesher.EncodeAddr(buf[1:19], msg.Src)
                        copy(buf[19:], msg.Buf[18:])
                        c.tx <- buf[:len(msg.Buf)+1]
                    }
                }
            }
        }
    }
}

func main() {
	port := ":8981"
	serve(port)
}
