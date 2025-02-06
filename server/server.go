package main

import (
    "log"
    "mesher/msg"
    "net"
    "time"
    "encoding/binary"
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

func server(rx chan net.UDPAddr, conn *net.UDPConn) {
    timed_out := make(chan net.UDPAddr)
    clients := make(map[string]chan struct{})
    clients_tx := make(map[string]chan []byte)
    ticker := time.NewTicker(5 * time.Second)
    go func(){
        for {
            select {
            case <- ticker.C:
                buf := make([]byte, 0)
                for k, _ := range clients {
                    buf = append(buf, k...)
                    buf = append(buf, ' ')
                }
                for _, c := range clients_tx {
                    c <- buf
                }
            case addr := <- rx:
                tx, ok := clients_tx[addr.String()]
                if !ok {
                    tx = writer(addr, conn)
                    clients_tx[addr.String()]=tx
                }
                c, ok := clients[addr.String()]
                if !ok {
                    c = watchdog(addr, timed_out)
                    clients[addr.String()]=c
                }
                c <- struct{}{}
            case a := <- timed_out:
                // todo delete client
                log.Println("Timed out ", a)
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

func reader(conn *net.UDPConn) chan net.UDPAddr {
    rx := make(chan net.UDPAddr)
    go func() {
        for {
            buf := make([]byte, 65536)
            n, addr, err := conn.ReadFromUDP(buf)
            if err != nil {
                log.Println("Reader Error: ", err)
                // todo really unrecoverable?
                break;
            }
            msg_type := binary.BigEndian.Uint32(buf[:n])
            switch msg_type {
            case msg.Client_hello:
                log.Println("Received Client_hello from ", addr)
                rx <- *addr
            }
        }
        log.Println("Reader Ending")
    }()
    return rx
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
    rx := reader(conn)
    server(rx, conn)

    for { }
    conn.Close()
}
