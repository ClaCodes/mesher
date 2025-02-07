package main

import (
    "log"
    "mesher/msg"
    "net"
    "time"
)

func watchdog(addr net.UDPAddr, timed_out chan net.UDPAddr) chan struct{} {
    channel := make(chan struct{})
    go func(){
        for {
            select {
            case <- channel:
            case <-time.After(5*time.Second):
                timed_out <- addr
                return
            }
        }
    }()
    return channel
}

type client struct {
    watchdog chan struct{}
    tx chan []byte
}

func server(hello_channel chan net.UDPAddr, relay_channel chan []byte, conn *net.UDPConn) {
    timed_out := make(chan net.UDPAddr)
    clients := make(map[msg.Mesher_addres] client)
    ticker := time.NewTicker(1 * time.Second)
    go func(){
        for {
            select {
            case <- ticker.C:
                buf := make([]byte, 0)
                buf = append(buf, msg.SERVER_HELLO)
                for k, _ := range clients {
                    buf = append(buf, k[:]...)
                }
                for addr, c := range clients {
                    log.Println("Sent     SERVER_HELLO to  ", msg.To_udp_addr(addr[:]), ":", buf[1:])
                    c.tx <- buf
                }
            case addr := <- hello_channel:
                var m msg.Mesher_addres
                msg.From_addr(m[:], addr)
                c, ok := clients[m]
                if !ok {
                    c = client{
                        watchdog(addr, timed_out),
                        writer(addr, conn),
                    }
                    clients[m] = c
                }
                c.watchdog <- struct{}{}
            case a := <- timed_out:
                var m msg.Mesher_addres
                msg.From_addr(m[:], a)
                delete(clients, m)
                // todo delete client
                log.Println("Timed out ", a)
            case b := <- relay_channel:
                // todo delete client
                if len(b) >= 18 {
                    dest := msg.To_udp_addr(b[:18])
                    log.Println("Relaying to ", dest, " ", b[18:])
                }
            }
        }
    }()
}

func writer(addr net.UDPAddr, conn *net.UDPConn) chan []byte{
    // todo goroutine is leaking if channel is not closed
    tx := make(chan []byte)
    go func() {
        for b := range tx {
            conn.WriteToUDP(b, &addr)
        }
        log.Println("Writer finishing for ", addr)
    }()
    return tx
}

func reader(conn *net.UDPConn) (chan net.UDPAddr, chan []byte) {
    hello_channel := make(chan net.UDPAddr)
    relay_channel := make(chan []byte)
    go func() {
        for {
            buf := make([]byte, 65536)
            n, addr, err := conn.ReadFromUDP(buf)
            if err != nil {
                log.Println("Reader Error: ", err)
                // todo really unrecoverable?
                break;
            }
            msg_type := buf[0]
            switch msg_type {
            case msg.CLIENT_HELLO:
                log.Println("Received CLIENT_HELLO from", addr)
                hello_channel <- *addr
            case msg.CLIENT_RELAY_TO:
                log.Println("Received CLIENT_RELAY_TO from", addr)
                relay_channel <- buf[1:n]
            }
        }
        log.Println("Reader Ending")
    }()
    return hello_channel, relay_channel
}

func main() {
    port := ":8981"
    local_addr, err := net.ResolveUDPAddr("udp", port)
    if err != nil {
        log.Fatal(err)
    }
    conn, err := net.ListenUDP("udp", local_addr)
    if err != nil {
        log.Fatal(err)
    }
    hello_channel, relay_channel := reader(conn)
    server(hello_channel, relay_channel, conn)

    for { }
    conn.Close()
}
