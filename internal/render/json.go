package render

import (
	"encoding/json"
	"io"

	"github.com/lucasew/go-hogkill/internal/proc"
)

type jsonProc struct {
	PID        int32   `json:"pid"`
	CPU        float64 `json:"cpu"`
	RSS        uint64  `json:"rss"`
	Elapsed    float64 `json:"elapsed"`
	Risk       string  `json:"risk"`
	RiskReason string  `json:"riskReason"`
	Command    string  `json:"command"`
}

type jsonGroup struct {
	Name       string     `json:"name"`
	User       string     `json:"user"`
	CPU        float64    `json:"cpu"`
	RSS        uint64     `json:"rss"`
	Risk       string     `json:"risk"`
	RiskReason string     `json:"riskReason"`
	Processes  []jsonProc `json:"processes"`
}

// WriteJSON emits groups as indented JSON.
func WriteJSON(w io.Writer, groups []proc.Group, top int) error {
	if top <= 0 || top > len(groups) {
		top = len(groups)
	}
	payload := make([]jsonGroup, 0, top)
	for _, g := range groups[:top] {
		jp := make([]jsonProc, 0, len(g.Procs))
		for _, p := range g.Procs {
			jp = append(jp, jsonProc{
				PID:        p.PID,
				CPU:        p.CPU,
				RSS:        p.RSS,
				Elapsed:    p.Elapsed,
				Risk:       string(p.Risk),
				RiskReason: p.RiskReason,
				Command:    p.Command,
			})
		}
		payload = append(payload, jsonGroup{
			Name:       g.Name,
			User:       g.User,
			CPU:        g.CPU,
			RSS:        g.RSS,
			Risk:       string(g.Risk),
			RiskReason: g.RiskReason,
			Processes:  jp,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
