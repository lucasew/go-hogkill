package proc

// RiskLevel is how much of the machine goes down with the process.
// "own" is hk itself or the terminal/shell it runs in.
type RiskLevel string

const (
	RiskNone     RiskLevel = "none"
	RiskSystem   RiskLevel = "system"
	RiskOwn      RiskLevel = "own"
	RiskCritical RiskLevel = "critical"
)

// RiskOrder ranks severity for worst-of aggregation.
var RiskOrder = map[RiskLevel]int{
	RiskNone:     0,
	RiskSystem:   1,
	RiskOwn:      2,
	RiskCritical: 3,
}

// RiskTag is the short column label.
var RiskTag = map[RiskLevel]string{
	RiskNone:     "",
	RiskSystem:   "system",
	RiskOwn:      "you",
	RiskCritical: "critical",
}

// RiskWord is used in confirm prompts.
var RiskWord = map[RiskLevel]string{
	RiskNone:     "",
	RiskSystem:   "system process",
	RiskOwn:      "your own session",
	RiskCritical: "CRITICAL system process",
}

func WorstRisk(a, b RiskLevel) RiskLevel {
	if RiskOrder[a] >= RiskOrder[b] {
		return a
	}
	return b
}

// Proc is one process after sampling and risk marking.
type Proc struct {
	PID        int32
	PPID       int32
	RSS        uint64 // bytes
	CPU        float64
	CPUSeconds float64
	Elapsed    float64 // seconds
	User       string
	Command    string
	Exe        string
	Name       string
	Risk       RiskLevel
	RiskReason string
}

// Group is one app row (folded processes).
type Group struct {
	Key        string
	Name       string
	Procs      []Proc
	CPU        float64
	RSS        uint64
	User       string
	Risk       RiskLevel
	RiskReason string
}

// Warning is a distinct consequence for a kill batch.
type Warning struct {
	Level  RiskLevel
	Name   string
	Reason string
}

// SortKey selects ranking.
type SortKey string

const (
	SortCPU   SortKey = "cpu"
	SortMem   SortKey = "mem"
	SortCount SortKey = "count"
	SortName  SortKey = "name"
)

// GroupOptions filters which processes fold into apps.
type GroupOptions struct {
	MinCPU   float64
	MinMem   uint64 // bytes
	User     string // empty = all
	Filter   string
	SafeOnly bool
}
