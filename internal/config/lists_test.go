// SPDX-License-Identifier: GPL-3.0-only

package config

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

// A list replaced while others read it: readers always see a whole list (run with -race).
func TestListReplacedWhileRead(t *testing.T) {
	bot := &BotConfig{Admins: []string{"alice!*@*"}}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 2000 {
				for _, mask := range List(&bot.Admins) {
					if mask == "" {
						t.Error("read a half-written list")
						return
					}
				}
			}
		}()
	}
	for i := range 500 {
		cur := List(&bot.Admins)
		if i%2 == 0 {
			SetList(&bot.Admins, append(cur, fmt.Sprintf("bob%d!*@*", i)))
		} else {
			SetList(&bot.Admins, slices.Delete(slices.Clone(cur), 0, 1))
		}
	}
	wg.Wait()
}

func TestSetListCopies(t *testing.T) {
	bot := &BotConfig{}
	in := []string{"alice", "bob"}
	SetList(&bot.ScreenNicks, in)
	in[0] = "mallory"
	if got := List(&bot.ScreenNicks); !slices.Equal(got, []string{"alice", "bob"}) {
		t.Errorf("list changed with the caller's slice: %q", got)
	}
	SetList(&bot.ScreenNicks, []string{})
	if List(&bot.ScreenNicks) == nil {
		t.Error("an emptied list became nil, so it would be saved as null and come back on restart")
	}
}
