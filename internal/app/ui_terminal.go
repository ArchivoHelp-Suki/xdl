package app

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

var termMu sync.Mutex

type interactiveControl struct {
	mu     sync.RWMutex
	paused bool
	quit   bool
}

func (c *interactiveControl) ShouldPause() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.paused
}

func (c *interactiveControl) ShouldQuit() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quit
}

func (c *interactiveControl) setPaused(v bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.paused = v
	c.mu.Unlock()
}

func (c *interactiveControl) setQuit() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.quit = true
	c.paused = false
	c.mu.Unlock()
}

var globalControl = &interactiveControl{}

func startKeyboardControlListener(c *interactiveControl) {
	if c == nil {
		return
	}
	go func() {
		r := bufio.NewReader(os.Stdin)
		for {
			ch, err := r.ReadByte()
			if err != nil {
				return
			}
			switch ch {
			case 'p', 'P':
				c.setPaused(true)
				termMu.Lock()
				fmt.Print("\rxdl> paused. press 'c' to continue or 'q' to quit.\n")
				termMu.Unlock()
				for {
					ch2, err2 := r.ReadByte()
					if err2 != nil {
						return
					}
					switch ch2 {
					case 'c', 'C':
						c.setPaused(false)
						termMu.Lock()
						fmt.Print("xdl> resuming...\n")
						termMu.Unlock()
						goto nextKey
					case 'q', 'Q':
						c.setQuit()
						termMu.Lock()
						fmt.Print("xdl! quit requested. finishing current cycle...\n")
						termMu.Unlock()
						return
					}
				}
			case 'q', 'Q':
				c.setQuit()
				termMu.Lock()
				fmt.Print("\rxdl! quit requested. finishing current cycle...\n")
				termMu.Unlock()
				return
			}
		nextKey:
		}
	}()
}

type spinner struct {
	label string
	stop  chan struct{}
	done  chan struct{}
}

func startSpinner(label string) *spinner {
	s := &spinner{
		label: label,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		frames := []rune{'|', '/', '-', '\\'}
		i := 0
		for {
			select {
			case <-s.stop:
				return
			default:
			}
			termMu.Lock()
			fmt.Printf("\r%s [%c]", s.label, frames[i%len(frames)])
			termMu.Unlock()
			i++
			time.Sleep(120 * time.Millisecond)
		}
	}()
	return s
}

func (s *spinner) Stop() {
	if s == nil {
		return
	}
	close(s.stop)
	<-s.done
	termMu.Lock()
	fmt.Print("\r")
	termMu.Unlock()
}

func buildProgressBar(width int, fraction float64) string {
	if width <= 0 {
		width = 20
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}

	filled := int(float64(width)*fraction + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}

	b := make([]byte, width)
	for i := 0; i < width; i++ {
		if i < filled {
			b[i] = '='
		} else {
			b[i] = ' '
		}
	}
	return string(b)
}
