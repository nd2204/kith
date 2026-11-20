package main

// PROTOTYPE — Variant A · Panels (lazygit-style, closest to the old vet45_env TUI).
// Left: stacked panels [1] Projects [2] Configs [3] Commands [4] Machines.
// Right: main panel [0] whose content follows the focused panel, and a Log strip.
// Overlays (edit, Conflict resolver, confirm) take over the main panel.
// Named commands run as embedded sessions inside the main panel (like the old Run mode).

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	aProjects = iota
	aConfigs
	aMain
	aCommands
	aMachines
)

type aMode int

const (
	aNone aMode = iota
	aEdit
	aAddKey
	aAddVal
	aConfirmUnset
	aResolve
	aResolveNew
	aMatrix // cursor inside the machine's grant matrix
)

type variantA struct {
	st                       *Store
	focus                    int
	proj, cfg, sec, cmd, mac int
	mode                     aMode
	input                    textinput.Model
	pendingKey               string
	resolveCur               int
	matrixCur                int
	reveal                   map[string]bool
	sessions                 []*session
	curSession               int
	flash                    string
}

func newA(st *Store) *variantA {
	ti := textinput.New()
	ti.Prompt = "› "
	return &variantA{st: st, input: ti, reveal: map[string]bool{}, focus: aMain}
}

func (v *variantA) Key() string  { return "A" }
func (v *variantA) Name() string { return "Panels (lazygit)" }
func (v *variantA) Capturing() bool {
	return v.mode == aEdit || v.mode == aAddKey || v.mode == aAddVal || v.mode == aResolveNew
}

func (v *variantA) p() *Project { return v.st.Projects[v.proj] }
func (v *variantA) c() *Config  { return v.p().Configs[min(v.cfg, len(v.p().Configs)-1)] }
func (v *variantA) s() *Secret {
	c := v.c()
	if !c.Granted || len(c.Secrets) == 0 {
		return nil
	}
	return c.Secrets[min(v.sec, len(c.Secrets)-1)]
}

// matrix rows for the selected machine: every (project, config) pair
func (v *variantA) matrixRows() [][2]int {
	var rows [][2]int
	for pi, p := range v.st.Projects {
		for ci := range p.Configs {
			rows = append(rows, [2]int{pi, ci})
		}
	}
	return rows
}

func (v *variantA) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case sessionTick:
		for _, s := range v.sessions {
			if s.id == m.id {
				return s.step()
			}
		}
		return nil
	case tea.KeyMsg:
		return v.key(m)
	}
	return nil
}

func move(cur, n, d int) int {
	if n == 0 {
		return 0
	}
	return (cur + d + n) % n
}

func (v *variantA) key(m tea.KeyMsg) tea.Cmd {
	k := m.String()
	v.flash = ""

	// --- overlays first ---
	switch v.mode {
	case aEdit, aAddKey, aAddVal, aResolveNew:
		switch k {
		case "esc":
			v.mode = aNone
			v.input.Blur()
			return nil
		case "enter":
			val := v.input.Value()
			v.input.Blur()
			switch v.mode {
			case aEdit:
				v.st.Set(v.p(), v.c(), v.pendingKey, val)
				v.mode = aNone
			case aAddKey:
				v.pendingKey = strings.ToUpper(val)
				v.mode = aAddVal
				v.input.SetValue("")
				v.input.Placeholder = "value or ${project.config.KEY}"
				return v.input.Focus()
			case aAddVal:
				v.st.Set(v.p(), v.c(), v.pendingKey, val)
				v.mode = aNone
			case aResolveNew:
				v.st.Resolve(v.p(), v.c(), v.pendingKey, val, "new value")
				v.mode = aNone
			}
			return nil
		}
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(m)
		return cmd
	case aConfirmUnset:
		if k == "y" {
			v.st.Unset(v.p(), v.c(), v.pendingKey)
			v.sec = max(0, v.sec-1)
		}
		v.mode = aNone
		return nil
	case aResolve:
		sec := v.s()
		n := len(sec.Conflict) + 1
		switch k {
		case "j", "down":
			v.resolveCur = move(v.resolveCur, n, 1)
		case "k", "up":
			v.resolveCur = move(v.resolveCur, n, -1)
		case "esc":
			v.mode = aNone
		case "enter":
			if v.resolveCur < len(sec.Conflict) {
				cd := sec.Conflict[v.resolveCur]
				v.st.Resolve(v.p(), v.c(), sec.Key, cd.Value, "kept "+cd.Machine+"'s value")
				v.mode = aNone
			} else {
				v.mode = aResolveNew
				v.input.SetValue("")
				v.input.Placeholder = "new value for " + sec.Key
				return v.input.Focus()
			}
		}
		return nil
	case aMatrix:
		rows := v.matrixRows()
		mac := v.st.Machines[v.mac]
		switch k {
		case "j", "down":
			v.matrixCur = move(v.matrixCur, len(rows), 1)
		case "k", "up":
			v.matrixCur = move(v.matrixCur, len(rows), -1)
		case "esc", "h":
			v.mode = aNone
		case " ", "space", "enter":
			r := rows[v.matrixCur]
			p := v.st.Projects[r[0]]
			c := p.Configs[r[1]]
			if !c.Granted && !mac.Has(p.Name, c.Name) {
				v.flash = "this Machine has no Grant on " + p.Name + "/" + c.Name + ": a Machine holding it must wrap the key (Admin signs anyway)"
			}
			v.st.ToggleGrant(mac, p, c)
		}
		return nil
	}

	// --- global ---
	switch k {
	case "tab":
		order := []int{aProjects, aConfigs, aMain, aCommands, aMachines}
		for i, f := range order {
			if f == v.focus {
				v.focus = order[(i+1)%len(order)]
				break
			}
		}
		return nil
	case "1":
		v.focus = aProjects
		return nil
	case "2":
		v.focus = aConfigs
		return nil
	case "0":
		v.focus = aMain
		return nil
	case "3":
		v.focus = aCommands
		return nil
	case "4":
		v.focus = aMachines
		return nil
	case "s":
		v.st.Sync()
		return nil
	}

	switch v.focus {
	case aProjects:
		switch k {
		case "j", "down":
			v.proj = move(v.proj, len(v.st.Projects), 1)
			v.cfg, v.sec, v.cmd = 0, 0, 0
		case "k", "up":
			v.proj = move(v.proj, len(v.st.Projects), -1)
			v.cfg, v.sec, v.cmd = 0, 0, 0
		case "enter", "l":
			v.focus = aConfigs
		}
	case aConfigs:
		switch k {
		case "j", "down":
			v.cfg = move(v.cfg, len(v.p().Configs), 1)
			v.sec = 0
		case "k", "up":
			v.cfg = move(v.cfg, len(v.p().Configs), -1)
			v.sec = 0
		case " ", "space":
			v.st.Use(v.p(), v.c())
		case "enter", "l":
			v.focus = aMain
		case "R":
			if v.c().Dropped > 0 {
				v.st.Reapply(v.p(), v.c())
			}
		}
	case aMain:
		c := v.c()
		switch k {
		case "j", "down":
			v.sec = move(v.sec, len(c.Secrets), 1)
		case "k", "up":
			v.sec = move(v.sec, len(c.Secrets), -1)
		case "v":
			if s := v.s(); s != nil {
				v.reveal[s.Key] = !v.reveal[s.Key]
			}
		case "V":
			all := !v.reveal["*"]
			v.reveal = map[string]bool{"*": all}
		case "a":
			if c.Granted {
				v.mode = aAddKey
				v.input.SetValue("")
				v.input.Placeholder = "KEY_NAME"
				return v.input.Focus()
			}
		case "e", "enter":
			s := v.s()
			if s == nil {
				return nil
			}
			if len(s.Conflict) > 0 && k == "enter" {
				v.mode, v.resolveCur = aResolve, 0
				return nil
			}
			v.mode, v.pendingKey = aEdit, s.Key
			v.input.SetValue(s.Value)
			if s.Ref != "" {
				v.input.SetValue(s.Ref)
			}
			return v.input.Focus()
		case "d":
			if s := v.s(); s != nil {
				v.mode, v.pendingKey = aConfirmUnset, s.Key
			}
		case "R":
			if c.Dropped > 0 {
				v.st.Reapply(v.p(), c)
			}
		}
	case aCommands:
		cmds := v.p().Commands
		switch k {
		case "j", "down":
			v.cmd = move(v.cmd, len(cmds), 1)
		case "k", "up":
			v.cmd = move(v.cmd, len(cmds), -1)
		case "enter":
			if len(cmds) == 0 {
				return nil
			}
			cmd := cmds[v.cmd]
			c := v.activeCfg()
			if !cmd.Allowed(c) {
				v.flash = fmt.Sprintf("%s only runs in %v; Active config is %s (%s)", cmd.Name, cmd.Envs, c.Name, c.Env)
				return nil
			}
			if miss := v.p().Missing(c); len(miss) > 0 {
				v.flash = "missing required keys: " + strings.Join(miss, ", ")
				return nil
			}
			s, tick := newSession(v.p(), c, cmd)
			v.sessions = append(v.sessions, s)
			v.curSession = len(v.sessions) - 1
			return tick
		case "]":
			v.curSession = move(v.curSession, len(v.sessions), 1)
		case "[":
			v.curSession = move(v.curSession, len(v.sessions), -1)
		case "x":
			if len(v.sessions) > 0 {
				v.sessions = append(v.sessions[:v.curSession], v.sessions[v.curSession+1:]...)
				v.curSession = max(0, v.curSession-1)
			}
		}
	case aMachines:
		switch k {
		case "j", "down":
			v.mac = move(v.mac, len(v.st.Machines), 1)
		case "k", "up":
			v.mac = move(v.mac, len(v.st.Machines), -1)
		case "enter", "l":
			mac := v.st.Machines[v.mac]
			if !mac.JoinRequest && !mac.Revoked {
				v.mode, v.matrixCur = aMatrix, 0
			}
		case "y":
			if mac := v.st.Machines[v.mac]; mac.JoinRequest {
				v.st.Approve(mac)
			}
		}
	}
	return nil
}

func (v *variantA) activeCfg() *Config {
	p := v.p()
	if c := p.Config(v.st.Active[p.Name]); c != nil {
		return c
	}
	return p.Configs[0]
}

// ---------- view ----------

func cfgBadges(c *Config, active bool) string {
	var b []string
	if active {
		b = append(b, sOK.Render("●"))
	}
	if !c.Granted {
		b = append(b, sDim.Render("no Grant"))
	}
	if n := c.Conflicts(); n > 0 {
		b = append(b, sWarn.Render(fmt.Sprintf("⚠%d", n)))
	}
	if c.PendingRotation != "" {
		b = append(b, sErr.Render("⟳"))
	}
	if c.Dropped > 0 {
		b = append(b, sErr.Render(fmt.Sprintf("✖%d", c.Dropped)))
	}
	return strings.Join(b, " ")
}

func machineStatus(m *Machine) (dot, status string) {
	dot = sDim.Render("○")
	status = sDim.Render("offline " + m.LastSeen)
	switch {
	case m.JoinRequest:
		dot, status = sWarn.Render("?"), sWarn.Render("join request")
	case m.Revoked:
		dot, status = sErr.Render("✖"), sErr.Render("revoked")
	case m.This:
		dot, status = sOK.Render("●"), sDim.Render("this machine")
	case m.Online && m.Behind > 0:
		dot, status = sOK.Render("●"), sWarn.Render(fmt.Sprintf("%d behind", m.Behind))
	case m.Online:
		dot, status = sOK.Render("●"), sOK.Render("synced")
	case m.Behind > 0:
		status = sDim.Render(fmt.Sprintf("offline %s · %d behind", m.LastSeen, m.Behind))
	}
	return
}

func machineLine(m *Machine) string {
	dot, status := machineStatus(m)
	return fmt.Sprintf("%s %-14s %s", dot, m.Name, status)
}

func (v *variantA) View(W, H int) string {
	bodyH := H - 1
	lw := max(34, W/3-4)
	rw := W - lw

	// left column
	var pl []string
	for i, p := range v.st.Projects {
		l := "  " + p.Name
		if p.Setup != "" {
			l += sDim.Render("  " + p.Setup)
		}
		if i == v.proj {
			l = sSel.Render("▸ "+p.Name) + sDim.Render(strings.TrimPrefix(l, "  "+p.Name))
		}
		pl = append(pl, l)
	}
	var cl []string
	p := v.p()
	for i, c := range p.Configs {
		name := c.Name
		if c.Root != "" {
			name = "  ↳ " + c.Name
		}
		l := fmt.Sprintf("%-14s %s", name, cfgBadges(c, v.st.Active[p.Name] == c.Name))
		if i == v.cfg {
			l = sSel.Render(fmt.Sprintf("%-14s", name)) + " " + cfgBadges(c, v.st.Active[p.Name] == c.Name)
		}
		cl = append(cl, l)
	}
	var kl []string
	for i, c := range p.Commands {
		l := fmt.Sprintf("%-11s %s", c.Name, sDim.Render(c.Desc))
		if len(c.Envs) > 0 {
			l = fmt.Sprintf("%-11s %s", c.Name, sDim.Render(fmt.Sprintf("%v", c.Envs)))
		}
		if i == v.cmd && v.focus == aCommands {
			l = sSel.Render(fmt.Sprintf("%-11s", c.Name)) + " " + sDim.Render(c.Desc)
		}
		kl = append(kl, l)
	}
	if len(kl) == 0 {
		kl = []string{sDim.Render("no Setup for this Project")}
	}
	var ml []string
	for i, m := range v.st.Machines {
		l := machineLine(m)
		if i == v.mac && v.focus == aMachines {
			l = sSel.Render(fit(l, lw-2))
		}
		ml = append(ml, l)
	}

	hp := len(pl) + 2
	hc := len(cl) + 2
	hk := len(kl) + 2
	hm := bodyH - hp - hc - hk
	left := strings.Join([]string{
		box("[1] Projects", pl, lw, hp, v.focus == aProjects),
		box("[2] Configs", cl, lw, hc, v.focus == aConfigs),
		box("[3] Commands", kl, lw, hk, v.focus == aCommands),
		box("[4] Machines", ml, lw, hm, v.focus == aMachines),
	}, "\n")

	// right column
	logH := 7
	mainH := bodyH - logH
	title, lines := v.mainView(rw - 2)
	main := box(title, lines, rw, mainH, v.focus == aMain || v.mode == aMatrix)
	from := max(0, len(v.st.Log)-(logH-2))
	var ll []string
	for _, l := range v.st.Log[from:] {
		ll = append(ll, colorLog(l))
	}
	right := main + "\n" + box("Log", ll, rw, logH, false)

	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	var out []string
	for i := 0; i < bodyH; i++ {
		a, b := "", ""
		if i < len(leftLines) {
			a = leftLines[i]
		}
		if i < len(rightLines) {
			b = rightLines[i]
		}
		out = append(out, fit(a, lw)+fit(b, rw))
	}
	out = append(out, v.footer())
	return strings.Join(out, "\n")
}

func colorLog(l string) string {
	switch {
	case strings.HasPrefix(l, "Conflict"):
		return sWarn.Render(l)
	case strings.HasPrefix(l, "Dropped"), strings.Contains(l, "revoke"):
		return sErr.Render(l)
	case strings.HasPrefix(l, "Join"):
		return sWarn.Render(l)
	case strings.HasPrefix(l, "Sync"):
		return sInfo.Render(l)
	}
	return l
}

func (v *variantA) footer() string {
	if v.flash != "" {
		return sWarn.Render(" " + v.flash)
	}
	switch v.mode {
	case aEdit, aAddKey, aAddVal, aResolveNew:
		return " " + keyhelp("enter", "write Change", "esc", "cancel")
	case aConfirmUnset:
		return sWarn.Render(fmt.Sprintf(" unset %s on %s/%s? ", v.pendingKey, v.p().Name, v.c().Name)) + keyhelp("y", "yes", "any", "no")
	case aResolve:
		return " " + keyhelp("j/k", "choose", "enter", "resolve (new Change pointing at both heads)", "esc", "later")
	case aMatrix:
		return " " + keyhelp("j/k", "move", "space", "grant/revoke (signed as Admin)", "esc", "back")
	}
	common := []string{"tab", "panel", "1-4/0", "jump", "s", "sync", "</>", "variant", "q", "quit"}
	switch v.focus {
	case aConfigs:
		return " " + keyhelp(append([]string{"space", "kith use", "enter", "open", "R", "re-apply dropped"}, common...)...)
	case aMain:
		return " " + keyhelp(append([]string{"v/V", "reveal", "e", "edit", "a", "add", "d", "unset", "enter", "resolve Conflict"}, common...)...)
	case aCommands:
		return " " + keyhelp(append([]string{"enter", "run with Active config", "[ ]", "session", "x", "close"}, common...)...)
	case aMachines:
		return " " + keyhelp(append([]string{"enter", "Grants", "y", "approve join"}, common...)...)
	}
	return " " + keyhelp(common...)
}

func (v *variantA) mainView(w int) (string, []string) {
	switch {
	case v.mode == aResolve || v.mode == aResolveNew:
		return v.resolveView()
	case v.focus == aMachines || v.mode == aMatrix:
		return v.machineView()
	case v.focus == aCommands:
		return v.commandView(w)
	}
	return v.secretsView(w)
}

func (v *variantA) secretsView(w int) (string, []string) {
	p, c := v.p(), v.c()
	title := fmt.Sprintf("[0] Secrets · %s / %s", p.Name, c.Name)
	if c.Root != "" {
		title += sDim.Render("  (Branch config of " + c.Root + ")")
	}
	var out []string
	if !c.Granted {
		return title, []string{"", sDim.Render("  This Machine has no Grant on " + p.Name + "/" + c.Name + "."),
			sDim.Render("  Ask an Admin (" + strings.Join(p.Admins, ", ") + ") to run: kith grant " + c.Name + " alice-mbp")}
	}
	if c.PendingRotation != "" {
		out = append(out, sErr.Render(fmt.Sprintf(" ⟳ %s revoked · data key rotation pending (next sync) · %d keys should be rotated", c.PendingRotation, len(c.RotationKeys()))))
	}
	if c.Dropped > 0 {
		out = append(out, sErr.Render(fmt.Sprintf(" ✖ dropped %d Changes from revoked %s · R to re-apply them as yours", c.Dropped, c.DroppedFrom)))
	}
	if n := c.Conflicts(); n > 0 {
		out = append(out, sWarn.Render(fmt.Sprintf(" ⚠ %d Conflict · run uses the newest value meanwhile · enter on the key to resolve", n)))
	}
	if miss := p.Missing(c); len(miss) > 0 && p.Setup != "" {
		out = append(out, sWarn.Render(" requires (kith.yaml) missing: "+strings.Join(miss, ", ")))
	}
	if len(out) > 0 {
		out = append(out, "")
	}
	kw := 20
	vw := max(10, w-kw-28)
	out = append(out, sDim.Render(fmt.Sprintf(" %-*s %-*s %s", kw, "KEY", vw, "VALUE", "SOURCE")))
	for i, s := range c.Secrets {
		val := mask(s.Value)
		if v.reveal[s.Key] || v.reveal["*"] {
			val = s.Value
		}
		src := ""
		switch {
		case s.Ref != "":
			src = sInfo.Render(s.Ref)
		case s.Inherited:
			src = sDim.Render("↳ " + c.Root)
		case s.Overridden:
			src = sAccent.Render("override")
		}
		flags := ""
		if len(s.Conflict) > 0 {
			flags += sWarn.Render(" ⚠ Conflict")
		}
		if s.NeedsRotation {
			flags += sErr.Render(" ⟳ rotate")
		}
		row := fmt.Sprintf(" %-*s %-*s ", kw, s.Key, vw, fit(val, vw))
		if i == v.sec && v.focus == aMain {
			row = sSel.Render(row)
		}
		out = append(out, row+src+flags)
	}
	if v.mode == aEdit || v.mode == aAddKey || v.mode == aAddVal {
		label := "edit " + v.pendingKey
		if v.mode == aAddKey {
			label = "new key"
		} else if v.mode == aAddVal {
			label = "value for " + v.pendingKey
		}
		out = append(out, "", sAccent.Render(" "+label+" (local Change, syncs later)"), " "+v.input.View())
	}
	return title, out
}

func (v *variantA) resolveView() (string, []string) {
	s := v.s()
	out := []string{
		sWarn.Render(fmt.Sprintf(" Conflict · %s on %s/%s", s.Key, v.p().Name, v.c().Name)),
		sDim.Render(" Two concurrent Changes set different values. `run` currently uses the first one (newer HLC)."),
		"",
	}
	for i, cd := range s.Conflict {
		l := fmt.Sprintf("   %-22s  from %-12s %s", cd.Value, cd.Machine, sDim.Render(cd.When))
		if i == 0 {
			l += sDim.Render("  (in use)")
		}
		if i == v.resolveCur {
			l = sSel.Render(" ▸" + l[2:])
		}
		out = append(out, l)
	}
	nl := "   ✎ enter a new value…"
	if v.resolveCur == len(s.Conflict) {
		nl = sSel.Render(" ▸ ✎ enter a new value…")
	}
	out = append(out, nl)
	if v.mode == aResolveNew {
		out = append(out, "", " "+v.input.View())
	}
	out = append(out, "", sDim.Render(" History"))
	for _, h := range s.History {
		out = append(out, sDim.Render("   "+h))
	}
	return "Resolve Conflict", out
}

func (v *variantA) machineView() (string, []string) {
	m := v.st.Machines[v.mac]
	out := []string{
		fmt.Sprintf(" %s  %s  Member %s  tailnet %s", sBold.Render(m.Name), sDim.Render(m.Fingerprint), m.Member, m.Addr),
		"",
	}
	if m.JoinRequest {
		out = append(out,
			sWarn.Render(" `kith join vet45-erp` request"),
			"   fingerprint shown on that machine must match: "+sBold.Render(m.Fingerprint),
			"   WhoIs: "+m.Addr+" → dan@ (tailnet)",
			"",
			" "+keyhelp("y", "approve as Member dan (signed Change, no Grants yet)", "n", "reject"))
		return "Machine · join request", out
	}
	if m.Revoked {
		out = append(out, sErr.Render(" revoked · its Changes outside the revocation's causal past are dropped"))
		return "Machine", out
	}
	out = append(out, sDim.Render(fmt.Sprintf(" %-16s %-12s %s", "PROJECT", "CONFIG", "GRANT")))
	for i, r := range v.matrixRows() {
		p := v.st.Projects[r[0]]
		c := p.Configs[r[1]]
		mark := sDim.Render("[ ]")
		if m.Has(p.Name, c.Name) {
			mark = sOK.Render("[x]")
		}
		note := ""
		if c.PendingRotation != "" {
			note = sErr.Render(" ⟳ rotation pending")
		}
		l := fmt.Sprintf(" %-16s %-12s %s%s", p.Name, c.Name, mark, note)
		if v.mode == aMatrix && i == v.matrixCur {
			l = sSel.Render(fmt.Sprintf(" %-16s %-12s ", p.Name, c.Name)) + mark + note
		}
		out = append(out, l)
	}
	if v.mode != aMatrix {
		out = append(out, "", sDim.Render(" enter to edit Grants"))
	}
	return "Machine · Grants", out
}

func (v *variantA) commandView(w int) (string, []string) {
	p := v.p()
	if len(p.Commands) == 0 {
		return "Commands", nil
	}
	cmd := p.Commands[v.cmd]
	c := v.activeCfg()
	var out []string
	if len(v.sessions) > 0 {
		var tabs []string
		for i, s := range v.sessions {
			t := " " + s.label + " "
			if !s.done {
				t = " ● " + s.label + " "
			}
			if i == v.curSession {
				t = sSel.Render(t)
			}
			tabs = append(tabs, t)
		}
		out = append(out, strings.Join(tabs, sDim.Render("│")), "")
		out = append(out, v.sessions[v.curSession].lines...)
		return "Sessions", out
	}
	out = append(out,
		" "+sBold.Render(cmd.Name)+"  "+sDim.Render(cmd.Desc),
		" run:  "+cmd.Run,
		" dir:  "+cmd.Dir,
	)
	if len(cmd.Envs) > 0 {
		out = append(out, fmt.Sprintf(" environments: %v", cmd.Envs))
	}
	out = append(out, "", fmt.Sprintf(" Active config: %s (%s)  · enter runs `kith run %s` here", c.Name, c.Env, cmd.Name))
	if !cmd.Allowed(c) {
		out = append(out, sErr.Render(" not allowed in "+c.Env))
	}
	return "Command", out
}
