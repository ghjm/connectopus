package file_cleaner

import (
	"crypto/rand"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/exit_handler"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"os"
)

type DeleteHandle string

var deleter = struct {
	handles syncro.Map[DeleteHandle, string]
	sigs    chan os.Signal
}{}

// DeleteOnExit does its best to delete this file when the program exits
func DeleteOnExit(filePath string) DeleteHandle {
	var id DeleteHandle
	deleter.handles.WorkWith(func(_h *map[DeleteHandle]string) {
		h := *_h
		for {
			b := make([]byte, 8)
			n, err := rand.Read(b)
			if err != nil {
				panic(fmt.Sprintf("error reading from random number generator: %s", err))
			}
			if n != len(b) {
				panic("error reading from random number generator: too few bytes read")
			}
			id = DeleteHandle(b)
			_, ok := h[id]
			if !ok {
				break
			}
		}
		h[id] = filePath
	})
	return id
}

// DeleteNow deletes the file now and removes it from the shutdown deletion list
func (dh DeleteHandle) DeleteNow() error {
	var err error
	deleter.handles.WorkWith(func(_h *map[DeleteHandle]string) {
		h := *_h
		f, ok := h[dh]
		if !ok {
			err = fmt.Errorf("invalid delete handle")
		} else {
			err = os.Remove(f)
			delete(h, dh)
		}
	})
	return err
}

// DeleteAllNow deletes all the files staged for deletion
func DeleteAllNow() {
	deleter.handles.WorkWith(func(_h *map[DeleteHandle]string) {
		h := *_h
		for id, f := range h {
			_ = os.Remove(f)
			delete(h, id)
		}
	})
}

func init() {
	exit_handler.AddExitFunc(DeleteAllNow)
}
