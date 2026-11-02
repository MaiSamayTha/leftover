package vault

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Event is one line of the history log: something moved into the vault,
// restored from it, or emptied for good. The log lets anyone check exactly
// what Leftover has done on their machine.
type Event struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Entry  string    `json:"entry"`
	Name   string    `json:"name"`
	Bytes  int64     `json:"bytes"`
	Paths  []string  `json:"paths"`
}

func (v *Vault) historyPath() string { return filepath.Join(v.Dir, "history.log") }

func (v *Vault) log(action string, e Entry) {
	ev := Event{At: v.now(), Action: action, Entry: e.ID, Name: e.Name, Bytes: e.Bytes, Paths: e.Paths}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile(v.historyPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(raw, '\n'))
}

// History returns the most recent events, newest first. Zero means all.
func (v *Vault) History(limit int) ([]Event, error) {
	f, err := os.Open(v.historyPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil {
			all = append(all, ev)
		}
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, sc.Err()
}
