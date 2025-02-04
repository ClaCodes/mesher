package main

import (
    "log"
    // "mesher/msg"
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

func feeder(channel chan net.UDPAddr) {
    timed_out := make(chan net.UDPAddr)
    clients := make(map[string]chan struct{})
    go func(){
        for {
            select {
            case a := <- channel:
                c, ok := clients[a.String()]
                if ok {
                    log.Println("Feeding ", a)
                } else {
                    log.Println("New Feeding ", a)
                    c = watchdog(a, timed_out)
                    clients[a.String()]=c
                }
                c <- struct{}{}
            case a := <- timed_out:
                log.Println("Timed out ", a)
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
    buf := make([]byte, 65536)
    c := make(chan net.UDPAddr)
    feeder(c)

    for {
        n, addr, err := conn.ReadFromUDP(buf)
        if err != nil {
            log.Println("Error: ", err)
            continue
        }
        log.Println("Received ", string(buf[0:n]), " from ", addr)
        c<-*addr
        // rx <- buf[0:n]
        conn.WriteToUDP(buf[0:n], addr)
    }
    conn.Close()
}
