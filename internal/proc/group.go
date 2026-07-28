package proc

import (
	"sort"
	"strings"
)

// GroupProcesses collapses the process table into one row per app.
func GroupProcesses(procs []Proc, opt GroupOptions) []Group {
	buckets := map[string][]Proc{}
	for _, p := range procs {
		if opt.User != "" && p.User != opt.User {
			continue
		}
		if opt.SafeOnly && p.Risk != RiskNone {
			continue
		}
		key := p.User + " " + GroupName(p.Command, p.Exe)
		buckets[key] = append(buckets[key], p)
	}

	needle := strings.ToLower(strings.TrimSpace(opt.Filter))
	groups := make([]Group, 0, len(buckets))
	for key, members := range buckets {
		name := key
		if i := strings.IndexByte(key, ' '); i >= 0 {
			name = key[i+1:]
		}
		if needle != "" && !matches(name, members, needle) {
			continue
		}

		var cpu float64
		var rss uint64
		risk := RiskNone
		reason := ""
		for _, p := range members {
			cpu += p.CPU
			rss += p.RSS
			if RiskOrder[p.Risk] > RiskOrder[risk] {
				risk = p.Risk
				reason = p.RiskReason
			}
		}
		if cpu < opt.MinCPU && rss < opt.MinMem {
			continue
		}

		sort.Slice(members, func(i, j int) bool {
			if members[i].CPU != members[j].CPU {
				return members[i].CPU > members[j].CPU
			}
			return members[i].RSS > members[j].RSS
		})

		user := ""
		if len(members) > 0 {
			user = members[0].User
		}
		groups = append(groups, Group{
			Key:        key,
			Name:       name,
			Procs:      members,
			CPU:        float64(int(cpu*10+0.5)) / 10,
			RSS:        rss,
			User:       user,
			Risk:       risk,
			RiskReason: reason,
		})
	}
	return groups
}

func matches(name string, members []Proc, needle string) bool {
	if strings.Contains(strings.ToLower(name), needle) {
		return true
	}
	for _, p := range members {
		if strings.Contains(strings.ToLower(p.Command), needle) {
			return true
		}
		if itoa(p.PID) == needle {
			return true
		}
	}
	return false
}

func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// SortGroups ranks groups by key.
func SortGroups(groups []Group, key SortKey) []Group {
	out := append([]Group(nil), groups...)
	switch key {
	case SortMem:
		sort.Slice(out, func(i, j int) bool {
			if out[i].RSS != out[j].RSS {
				return out[i].RSS > out[j].RSS
			}
			return out[i].CPU > out[j].CPU
		})
	case SortCount:
		sort.Slice(out, func(i, j int) bool {
			if len(out[i].Procs) != len(out[j].Procs) {
				return len(out[i].Procs) > len(out[j].Procs)
			}
			return out[i].RSS > out[j].RSS
		})
	case SortName:
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	default: // cpu
		sort.Slice(out, func(i, j int) bool {
			if out[i].CPU != out[j].CPU {
				return out[i].CPU > out[j].CPU
			}
			return out[i].RSS > out[j].RSS
		})
	}
	return out
}

// CollectWarnings one warning per distinct consequence, worst first.
func CollectWarnings(procs []Proc) []Warning {
	seen := map[string]Warning{}
	for _, p := range procs {
		if p.Risk == RiskNone {
			continue
		}
		k := string(p.Risk) + ":" + p.RiskReason
		if _, ok := seen[k]; !ok {
			seen[k] = Warning{Level: p.Risk, Name: p.Name, Reason: p.RiskReason}
		}
	}
	out := make([]Warning, 0, len(seen))
	for _, w := range seen {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		return RiskOrder[out[i].Level] > RiskOrder[out[j].Level]
	})
	return out
}

// HighestRisk returns the worst risk among procs.
func HighestRisk(procs []Proc) RiskLevel {
	worst := RiskNone
	for _, p := range procs {
		worst = WorstRisk(worst, p.Risk)
	}
	return worst
}
