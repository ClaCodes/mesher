# mesher
Demo of simple netcode in the presence of NAT

## Concept
The server acts as a signaling and TURN-like server. The clients periodically
ask the `peerList` from the server. The server maintains a list of clients,
that have recently requested the `peerList`. It respons on these requests with
the list. The clients start periodically pinging each other with `keepAlive`
messages. In response they reply with `isAlive`. If NAT traversal is successful,
the `isAlive` can reach the original sender. In that case the clients start
sending the data-packages directly to each other. Until then or if NAT
traversal fails, they use the server as a relay and send their data-packages
to it for relaying to the peer instead. The client application has 3 channels.
A `done` channel, an `incoming` channel and a `broadcast` channel. On the
`done` channel the client will notify, that the netcode is shutting down.
On the `incoming` channel incoming data-packages will be send to the
client application with an id representing the peer. The id starts at 0 and
counts up. On the `broadcast` channel, the application may send data-packages
to all peers.

## Demo app
The demo app opens a window using the [raylib](https://www.raylib.com/)
library. It then collects 'right-clicks' and sends them to the peer. The state
is updated in a lock-step fasion. So that all peers just send the input to
eachother and update the state independently. The state-update has an
artificially introduced lag to allow for network delay without halting the
simulation.

## Running
For now it was only tested locally. Run a server instance and run two client
instances. Right-click in window of client 1 blues squares should appear.
Red squares should appear in the window of client 2 and vice versa.
### Server
The server can be run directly on the machine by using go:
```
go run mesher/server
```
Alternatively it can be run in a container by using the provided `Containerfile`:
```
podman build -t mesher_server:latest .
podman run -d --rm -p 8981:8981/udp mesher_server:latest
```
### Client
The client pulls in the entire raylib library. The first build/run will take
much longer.
```
go run mesher/client
```
