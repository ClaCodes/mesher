package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"mesher/mesher"
	"time"
	"log"
	"encoding/gob"
	"bytes"
)

const simulationPeriod = 100 * time.Millisecond

type rectangle struct {
	fraction int
	live int
	pos rl.Vector2
}

type state struct {
	rectangles []rectangle
}

type action struct {
	Pos rl.Vector2
}

func (s *state)simulate(actions, fromPeer []action) {
	rs := make([]rectangle, 0)
	for _,a := range s.rectangles {
		a.live -= 1
		if a.live > 0 {
			rs = append(rs, a)
		}
	}
	s.rectangles = rs
	for _,a := range actions {
		s.rectangles = append(s.rectangles, rectangle{0, 10, a.Pos})
	}
	for _,a := range fromPeer {
		s.rectangles = append(s.rectangles, rectangle{1, 10, a.Pos})
	}
}

func simulator(actions chan action, fromPeer, toPeer chan []action) chan state {
	states := make(chan state, 1)
	go func() {
		collectedActions := make([]action, 0)
		peer := make([]action, 0)
		actionHistory := make([][]action, 0)
		actionHistory = append(actionHistory, make([]action, 0))
		ticker := time.NewTicker(simulationPeriod)
		s := state {
			make([]rectangle, 0),
		}
		for {
			select {
			case a := <-fromPeer:
				peer = a
			case a := <-actions:
				collectedActions = append(collectedActions, a)
			case <-ticker.C:
				var current []action
				current, actionHistory = actionHistory[0], append(actionHistory[1:], collectedActions)
				toPeer <- collectedActions
				collectedActions = make([]action, 0)
				s.simulate(current, peer)
				/* TODO slow down in case peer action missing */
				peer = make([]action, 0)
				states <- s
			}
		}
	}()
	return states
}

func communicator(toPeer chan []action) chan []action{
	fromPeer := make(chan []action, 1)
	go func () {
		for {
			address := "127.0.0.1:8981"
			broadcast, done, incoming := mesher.Bonder(address)

			pollLoop: for {
				select {
				case d:=<-toPeer:
					var b bytes.Buffer
					enc := gob.NewEncoder(&b)
					err := enc.Encode(d)
					if err != nil {
						log.Fatal("encode:", err)
					}
					broadcast <- b.Bytes()
				case m:=<-incoming:
					var b bytes.Buffer
					b.Write(m.Buf)
					dec := gob.NewDecoder(&b)
					var a []action
					err := dec.Decode(&a)
					if err != nil {
						log.Fatal("decode:", err)
					}
					if len(a) > 0 {
						log.Println(m.PeerId, a)
					}
					fromPeer<-a
				case <-done:
					break pollLoop;
				}
			}
		}
	}()
	return fromPeer
}

func main() {
	actions := make(chan action, 1)
	toPeer := make(chan []action, 1)
	fromPeer := communicator(toPeer)
	states := simulator(actions, fromPeer, toPeer)
	rl.SetConfigFlags(rl.FlagWindowHighdpi)
	rl.SetConfigFlags(rl.FlagWindowResizable)
	rl.InitWindow(800, 450, "raylib [core] example - basic window")
	defer rl.CloseWindow()

	reset := false

	rl.SetTargetFPS(60)
	s:=state{}

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.ClearBackground(rl.RayWhite)

		mouseUp := rl.IsMouseButtonDown(rl.MouseButtonRight)
		mouseDown := rl.IsMouseButtonUp(rl.MouseButtonRight)
		if mouseDown && reset {
			actions <- action{rl.GetMousePosition()}
			reset = false
		} else if mouseUp {
			reset = true
		}

		drainLoop:
		for {
			select {
			case s=<-states:
			default:
				break drainLoop
			}
		}

		for _,v := range s.rectangles {
			/* TODO strange *2 workaround needed for high dpi screen 8 */
			c := rl.Blue
			if v.fraction != 0 {
				c = rl.Red
			}
			rl.DrawRectangle(int32(v.pos.X)*2, int32(v.pos.Y)*2, 50, 50, c)
		}

		rl.EndDrawing()
	}
}

