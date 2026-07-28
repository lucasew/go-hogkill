package proc

import (
	"os"
	"runtime"
	"strings"
)

type knownRisk struct {
	Level  RiskLevel
	Reason string
}

var darwinRisk = map[string]knownRisk{
	"kernel_task":      {RiskCritical, "the kernel itself; the machine panics"},
	"launchd":          {RiskCritical, "PID 1, parent of everything; macOS panics"},
	"WindowServer":     {RiskCritical, "draws every window; logs you out instantly"},
	"loginwindow":      {RiskCritical, "owns your login session; ends it"},
	"configd":          {RiskCritical, "runs the network stack; networking dies"},
	"powerd":           {RiskCritical, "power management; sleep, wake and battery break"},
	"opendirectoryd":   {RiskCritical, "resolves users and groups; logins start failing"},
	"securityd":        {RiskCritical, "keychain and crypto; apps lose credentials"},
	"amfid":            {RiskCritical, "validates code signatures; apps stop launching"},
	"runningboardd":    {RiskCritical, "manages app lifecycles; apps get suspended or killed"},
	"launchservicesd":  {RiskCritical, "launches apps and opens files; double-click stops working"},
	"kernelmanagerd":   {RiskCritical, "loads kernel extensions; drivers break"},
	"watchdogd":        {RiskCritical, "the system watchdog; may force a reboot"},
	"hidd":             {RiskCritical, "keyboard, mouse and trackpad input"},
	"diskarbitrationd": {RiskCritical, "mounts disks; volumes stop appearing"},
	"notifyd":          {RiskSystem, "system-wide notifications between processes"},
	"distnoted":        {RiskSystem, "distributed notifications; apps lose sync"},
	"syslogd":          {RiskSystem, "system logging stops"},
	"coreaudiod":       {RiskSystem, "all audio dies until it restarts"},
	"bluetoothd":       {RiskSystem, "bluetooth drops; including keyboard and mouse"},
	"mDNSResponder":    {RiskSystem, "DNS resolution; name lookups fail until it restarts"},
	"Dock":             {RiskSystem, "the Dock and Mission Control (relaunches on its own)"},
	"Finder":           {RiskSystem, "the desktop and file windows (relaunches on its own)"},
	"SystemUIServer":   {RiskSystem, "menu bar extras (relaunches on its own)"},
	"WindowManager":    {RiskSystem, "Stage Manager and window tiling"},
	"trustd":           {RiskSystem, "certificate validation; TLS starts failing"},
	"syspolicyd":       {RiskSystem, "Gatekeeper checks; apps may refuse to open"},
	"fseventsd":        {RiskSystem, "filesystem change events; Spotlight and sync tools go blind"},
	"cfprefsd":         {RiskSystem, "reads and writes app preferences"},
}

var linuxRisk = map[string]knownRisk{
	"systemd":          {RiskCritical, "PID 1 and service manager; the system goes down"},
	"init":             {RiskCritical, "PID 1; the system goes down"},
	"kthreadd":         {RiskCritical, "parent of every kernel thread"},
	"systemd-journald": {RiskCritical, "system logging; services may block on it"},
	"systemd-logind":   {RiskCritical, "owns login sessions; you get logged out"},
	"systemd-udevd":    {RiskCritical, "device management; hotplug and drivers break"},
	"dbus-daemon":      {RiskCritical, "desktop IPC bus; the session falls apart"},
	"udevd":            {RiskCritical, "device management; hotplug and drivers break"},
	"sshd":             {RiskCritical, "the SSH server; remote sessions die, including yours"},
	"Xorg":             {RiskCritical, "the X server; the graphical session ends"},
	"Xwayland":         {RiskCritical, "X compatibility layer; X apps die"},
	"gnome-shell":      {RiskCritical, "the desktop shell; the session ends"},
	"plasmashell":      {RiskCritical, "the desktop shell; the session ends"},
	"gdm":              {RiskCritical, "the display manager; you get logged out"},
	"sddm":             {RiskCritical, "the display manager; you get logged out"},
	"lightdm":          {RiskCritical, "the display manager; you get logged out"},
	"NetworkManager":   {RiskCritical, "networking; connections drop"},
	"wpa_supplicant":   {RiskSystem, "wi-fi authentication; wireless drops"},
	"pipewire":         {RiskSystem, "audio and video routing; sound dies"},
	"pulseaudio":       {RiskSystem, "audio server; sound dies"},
}

var windowsRisk = map[string]knownRisk{
	"System":                {RiskCritical, "the Windows kernel; the machine bugchecks"},
	"Registry":              {RiskCritical, "the registry process; Windows bugchecks"},
	"smss":                  {RiskCritical, "the session manager; instant blue screen"},
	"csrss":                 {RiskCritical, "the Win32 subsystem; instant blue screen"},
	"wininit":               {RiskCritical, "starts every system service; instant blue screen"},
	"winlogon":              {RiskCritical, "owns your logon session; you get signed out"},
	"services":              {RiskCritical, "the service control manager; Windows bugchecks"},
	"lsass":                 {RiskCritical, "local security authority; instant blue screen"},
	"LsaIso":                {RiskCritical, "credential guard; instant blue screen"},
	"dwm":                   {RiskCritical, "the desktop compositor; the screen goes black"},
	"svchost":               {RiskSystem, "hosts Windows services; networking or audio may drop"},
	"explorer":              {RiskSystem, "taskbar, desktop and file windows (restarts on its own)"},
	"fontdrvhost":           {RiskSystem, "font rendering for the session"},
	"audiodg":               {RiskSystem, "audio device graph; sound dies until it restarts"},
	"spoolsv":               {RiskSystem, "the print spooler; printing stops"},
	"WmiPrvSE":              {RiskSystem, "WMI provider host; process tools read through WMI"},
	"SearchIndexer":         {RiskSystem, "Windows Search; results go stale until it restarts"},
	"RuntimeBroker":         {RiskSystem, "brokers permissions for Store apps"},
	"sihost":                {RiskSystem, "shell infrastructure; parts of the UI stop responding"},
	"ctfmon":                {RiskSystem, "text input and IME"},
	"taskhostw":             {RiskSystem, "host for scheduled background tasks"},
	"conhost":               {RiskSystem, "console window host; a terminal window may close"},
	"MsMpEng":               {RiskSystem, "Microsoft Defender antivirus engine"},
	"SecurityHealthService": {RiskSystem, "Windows Security monitoring"},
}

var systemUsers = map[string]struct{}{
	"root": {}, "daemon": {}, "nobody": {},
	"systemd-network": {}, "systemd-resolve": {},
}

func riskTable() map[string]knownRisk {
	switch runtime.GOOS {
	case "darwin":
		return darwinRisk
	case "windows":
		return windowsRisk
	default:
		return linuxRisk
	}
}

// AssessRisk explains what a process is worth before you kill it.
func AssessRisk(p Proc, lineage map[int32]struct{}) (RiskLevel, string) {
	if p.PID == int32(os.Getpid()) {
		return RiskOwn, "this is hk itself"
	}
	if p.PID <= 1 {
		return RiskCritical, "PID 1; the whole system hangs off it"
	}

	table := riskTable()
	exe := BaseName(p.Exe)
	if k, ok := table[exe]; ok {
		return k.Level, k.Reason
	}
	if k, ok := table[p.Name]; ok {
		return k.Level, k.Reason
	}

	if _, ok := lineage[p.PID]; ok {
		return RiskOwn, "the shell or terminal running hk; this session closes"
	}

	if _, ok := systemUsers[p.User]; ok || strings.HasPrefix(p.User, "_") {
		return RiskSystem, "system daemon running as " + p.User + "; something in the OS depends on it"
	}
	return RiskNone, ""
}
