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
            case <- time.After(5*time.Second):
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

func server(client_msg_channel chan msg.Mesher_msg, conn *net.UDPConn) {
    timed_out := make(chan net.UDPAddr)
    clients := make(map[msg.Mesher_addres] client)
    ticker := time.NewTicker(1 * time.Second)
    go func(){
        for {
            select {
            case <- ticker.C:
                for addr, c := range clients {
                    buf := make([]byte, 0)
                    buf = append(buf, msg.SERVER_HELLO)
                    for k, _ := range clients {
                        if k != addr {
                            buf = append(buf, k[:]...)
                        }
                    }
                    log.Println("Sent     SERVER_HELLO to  ", msg.To_udp_addr(addr[:]), ":", buf[1:])
                    c.tx <- buf
                }
            case a := <- timed_out:
                var m msg.Mesher_addres
                msg.From_addr(m[:], a)
                c, ok := clients[m]
                if ok {
                    close(c.tx)
                    delete(clients, m)
                }
                log.Println("Timed out ", a)
            case client_msg := <- client_msg_channel:
                switch client_msg.Msg_type {
                case msg.CLIENT_HELLO:
                    log.Println("Received CLIENT_HELLO from", client_msg.Src)
                    var m msg.Mesher_addres
                    msg.From_addr(m[:], client_msg.Src)
                    c, ok := clients[m]
                    if !ok {
                        tx := make(chan []byte)
                        msg.Writer_to(client_msg.Src, conn, tx)
                        c = client {
                            watchdog(client_msg.Src, timed_out),
                            tx,
                        }
                        clients[m] = c
                    }
                    c.watchdog <- struct{}{}
                case msg.CLIENT_RELAY_TO:
                    log.Println("Received CLIENT_RELAY_TO from", client_msg.Src)
                    if len(client_msg.Buf) >= 18 {
                        var m msg.Mesher_addres
                        dst := msg.To_udp_addr(client_msg.Buf[:18])
                        msg.From_addr(m[:], dst)
                        c, ok := clients[m]
                        if !ok {
                            log.Println("Relaying from", client_msg.Src, "to", dst, "but client not present");
                        } else {
                            buf := make([]byte, 65536)
                            buf[0] = msg.SERVER_RELAY_FROM
                            msg.From_addr(buf[1:19], client_msg.Src)
                            copy(buf[19:], client_msg.Buf[18:])
                            c.tx <- buf[:len(client_msg.Buf) + 1]
                        }
                    }
                }
            }
        }
    }()
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
    client_msg_channel := msg.Reader(conn)
    server(client_msg_channel, conn)

    for { }
    conn.Close()
}
