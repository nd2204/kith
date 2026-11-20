package main

// PROTOTYPE — Variant C · Setup tabs (directory-centred control panel).
// Opens on the Project of the nearest kith.yaml. The Config chips row *is* the
// Active config: `[`/`]` runs `kith use`. Four full-width tabs:
//   1 Secrets   list + inspector (value, source, history, Conflict resolved inline)
//   2 Commands  Named commands with requires/environments checks, output split below
//   3 Access    Machine × Config grant matrix grouped by Member, join requests on top
//   4 Activity  "needs you" items (actionable) above the Change feed

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type variantC struct {
	st       *Store
	proj     int
	tab      int
	sec      int
	cand     int  // cursor in the Conflict candidates
	inCand   bool // focus is inside the inspector's Conflict chooser
	cmd      int
	out      *session
	row, col int // Access matrix cursor
	act      int // Activity cursor
	reveal   bool
	editing  string // "", "edit", "add-key", "add-val", "new"
	pend     string
	input    textinput.Model
	flash    string
}

func newC(st *Store) *variantC {
	ti := textinput.New()
	ti.Prompt = "› "
	return &variantC{st: st, input: ti}
}

func (v *variantC) Key() string     { return "C" }
func (v *variantC) Name() string    { return "Setup tabs" }
func (v *variantC) Capturing() bool { return v.editing != "" }

func (v *variantC) p() *Project { return v.st.Projects[v.proj] }
func (v *variantC) c() *Config {
	p := v.p()
	if c := p.Config(v.st.Active[p.Name]); c != nil {
		return c
	}
	return p.Configs[0]
}
func (v *variantC) s() *Secret {
	c := v.c()
	if !c.Granted || len(c.Secrets) == 0 {
		return nil
	}
	return c.Secrets[min(v.sec, len(c.Secrets)-1)]
}

// access matrix rows: machines sorted by member, join requests excluded
func (v *variantC) accessRows() []*Machine {
	var rows []*Machine
	for _, m := range v.st.Machines {
		if !m.JoinRequest {
			rows = append(rows, m)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Member < rows[j].Member })
	return rows
}

type cItem struct {
	text string
	do   func()
}

func (v *variantC) needsYou() []cItem {
	var it []cItem
	for pi, p := range v.st.Projects {
		for _, c := range p.Configs {
			if !c.Granted {
				continue
			}
			pi, p, c := pi, p, c
			for si, s := range c.Secrets {
				if len(s.Conflict) > 0 {
					si, s := si, s
					it = append(it, cItem{sWarn.Render(fmt.Sprintf("⚠ Conflict  %s/%s %s · %s vs %s", p.Name, c.Name, s.Key, s.Conflict[0].Machine, s.Conflict[1].Machine)),
						func() {
							v.proj, v.tab, v.sec, v.inCand = pi, 0, si, true
							v.st.Active[p.Name] = c.Name
						}})
				}
			}
			if c.Dropped > 0 {
				it = append(it, cItem{sErr.Render(fmt.Sprintf("✖ Dropped   %d Changes from revoked %s on %s/%s · enter re-applies them as yours", c.Dropped, c.DroppedFrom, p.Name, c.Name)),
					func() { v.st.Reapply(p, c) }})
			}
			if c.PendingRotation != "" {
				it = append(it, cItem{sErr.Render(fmt.Sprintf("⟳ Rotation  %s/%s data key waits for next sync (revoked %s) · rotate values: %s", p.Name, c.Name, c.PendingRotation, strings.Join(c.RotationKeys(), ", "))),
					func() { v.st.Sync() }})
			} else if ks := c.RotationKeys(); len(ks) > 0 {
				it = append(it, cItem{sWarn.Render(fmt.Sprintf("⟳ Exposed   %s/%s values seen by a revoked Machine: %s · enter opens the first", p.Name, c.Name, strings.Join(ks, ", "))),
					func() {
						v.proj, v.tab = pi, 0
						v.st.Active[p.Name] = c.Name
						for i, s := range c.Secrets {
							if s.NeedsRotation {
								v.sec = i
								break
							}
						}
					}})
			}
		}
	}
	for _, m := range v.st.Machines {
		if m.JoinRequest {
			m := m
			it = append(it, cItem{sWarn.Render(fmt.Sprintf("? Join      %s (Member %s) fingerprint %s · enter approves", m.Name, m.Member, m.Fingerprint)),
				func() { v.st.Approve(m) }})
		}
	}
	return it
}

func (v *variantC) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case sessionTick:
		if v.out != nil && v.out.id == m.id {
			return v.out.step()
		}
	case tea.KeyMsg:
		return v.key(m)
	}
	return nil
}

func (v *variantC) startInput(mode, prompt, val string) tea.Cmd {
	v.editing = mode
	v.input.Prompt = prompt
	v.input.SetValue(val)
	return v.input.Focus()
}

func (v *variantC) key(m tea.KeyMsg) tea.Cmd {
	k := m.String()
	v.flash = ""
	p, c := v.p(), v.c()

	if v.editing != "" {
		switch k {
		case "esc":
			v.editing = ""
			v.input.Blur()
			return nil
		case "enter":
			val := v.input.Value()
			mode := v.editing
			v.editing = ""
			v.input.Blur()
			switch mode {
			case "edit":
				v.st.Set(p, c, v.pend, val)
			case "add-key":
				v.pend = strings.ToUpper(val)
				return v.startInput("add-val", v.pend+" = ", "")
			case "add-val":
				v.st.Set(p, c, v.pend, val)
			case "new":
				v.st.Resolve(p, c, v.pend, val, "new value")
				v.inCand = false
			}
			return nil
		}
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(m)
		return cmd
	}

	switch k {
	case "1", "2", "3", "4":
		v.tab = int(k[0] - '1')
		v.inCand = false
		return nil
	case "tab":
		v.tab = (v.tab + 1) % 4
		v.inCand = false
		return nil
	case "shift+tab":
		v.tab = (v.tab + 3) % 4
		return nil
	case "[", "]":
		d := 1
		if k == "[" {
			d = -1
		}
		idx := 0
		for i, cc := range p.Configs {
			if cc == c {
				idx = i
			}
		}
		v.st.Use(p, p.Configs[move(idx, len(p.Configs), d)])
		v.sec, v.inCand = 0, false
		return nil
	case "p":
		v.proj = move(v.proj, len(v.st.Projects), 1)
		v.sec, v.cmd, v.inCand = 0, 0, false
		return nil
	case "s":
		v.st.Sync()
		return nil
	}

	switch v.tab {
	case 0: // Secrets
		sec := v.s()
		if v.inCand && sec != nil && len(sec.Conflict) > 0 {
			n := len(sec.Conflict) + 1
			switch k {
			case "j", "down":
				v.cand = move(v.cand, n, 1)
			case "k", "up":
				v.cand = move(v.cand, n, -1)
			case "esc", "h", "left":
				v.inCand = false
			case "enter":
				if v.cand < len(sec.Conflict) {
					cd := sec.Conflict[v.cand]
					v.st.Resolve(p, c, sec.Key, cd.Value, "kept "+cd.Machine+"'s value")
					v.inCand = false
				} else {
					v.pend = sec.Key
					return v.startInput("new", "new "+sec.Key+" = ", "")
				}
			}
			return nil
		}
		switch k {
		case "j", "down":
			v.sec = move(v.sec, len(c.Secrets), 1)
		case "k", "up":
			v.sec = move(v.sec, len(c.Secrets), -1)
		case "v":
			v.reveal = !v.reveal
		case "enter", "l", "right":
			if sec != nil && len(sec.Conflict) > 0 {
				v.inCand, v.cand = true, 0
			} else if sec != nil {
				v.pend = sec.Key
				return v.startInput("edit", sec.Key+" = ", sec.Value)
			}
		case "e":
			if sec != nil {
				v.pend = sec.Key
				val := sec.Value
				if sec.Ref != "" {
					val = sec.Ref
				}
				return v.startInput("edit", sec.Key+" = ", val)
			}
		case "a":
			if c.Granted {
				return v.startInput("add-key", "new key: ", "")
			}
		case "d":
			if sec != nil {
				v.st.Unset(p, c, sec.Key)
				v.sec = max(0, v.sec-1)
			}
		}
	case 1: // Commands
		switch k {
		case "j", "down":
			v.cmd = move(v.cmd, len(p.Commands), 1)
		case "k", "up":
			v.cmd = move(v.cmd, len(p.Commands), -1)
		case "enter":
			if len(p.Commands) == 0 {
				return nil
			}
			cmd := p.Commands[v.cmd]
			if !cmd.Allowed(c) {
				v.flash = fmt.Sprintf("%s only runs in %v", cmd.Name, cmd.Envs)
				return nil
			}
			if miss := p.Missing(c); len(miss) > 0 {
				v.flash = "missing required keys: " + strings.Join(miss, ", ")
				return nil
			}
			s, tick := newSession(p, c, cmd)
			v.out = s
			return tick
		case "x":
			v.out = nil
		}
	case 2: // Access
		rows := v.accessRows()
		switch k {
		case "j", "down":
			v.row = move(v.row, len(rows), 1)
		case "k", "up":
			v.row = move(v.row, len(rows), -1)
		case "l", "right":
			v.col = move(v.col, len(p.Configs), 1)
		case "h", "left":
			v.col = move(v.col, len(p.Configs), -1)
		case " ", "space", "enter":
			m := rows[v.row]
			if m.Revoked {
				v.flash = m.Name + " is revoked"
				return nil
			}
			v.st.ToggleGrant(m, p, p.Configs[v.col])
		case "y":
			for _, m := range v.st.Machines {
				if m.JoinRequest {
					v.st.Approve(m)
					break
				}
			}
		}
	case 3: // Activity
		items := v.needsYou()
		switch k {
		case "j", "down":
			v.act = move(v.act, len(items), 1)
		case "k", "up":
			v.act = move(v.act, len(items), -1)
		case "enter":
			if v.act < len(items) {
				items[v.act].do()
				v.act = 0
			}
		}
	}
	return nil
}

// ---------- view ----------

func (v *variantC) View(W, H int) string {
	p, c := v.p(), v.c()
	this := v.st.This()
	on, total := v.st.Online()

	where := p.Setup
	if where == "" {
		where = "no kith.yaml"
	}
	head := sAccent.Render(" "+p.Name) + sDim.Render("  "+where+"  (p: next Project)")
	right := sDim.Render(fmt.Sprintf("%s · peers %d/%d · synced %s ", this.Name, on, total, v.st.LastSync))
	out := []string{head + strings.Repeat(" ", max(1, W-lenVisible(head)-lenVisible(right))) + right}

	// Config chips = Active config
	var chips []string
	for _, cc := range p.Configs {
		label := cc.Name
		if b := cfgBadges(cc, false); b != "" {
			label += " " + b
		}
		if cc.Root != "" {
			label = "↳" + label
		}
		if cc == c {
			chips = append(chips, sSel.Render(" "+label+" "))
		} else {
			chips = append(chips, sDim.Render(" ")+label+sDim.Render(" "))
		}
	}
	out = append(out, " Active config "+strings.Join(chips, sDim.Render("│"))+sDim.Render("   [ ] kith use"))

	// tabs
	needs := len(v.needsYou())
	names := []string{"Secrets", "Commands", "Access", fmt.Sprintf("Activity (%d)", needs)}
	var tabs []string
	for i, n := range names {
		t := fmt.Sprintf(" %d %s ", i+1, n)
		if i == v.tab {
			tabs = append(tabs, sAccent.Render("▔"+t+"▔"))
		} else {
			tabs = append(tabs, sDim.Render(" "+t+" "))
		}
	}
	out = append(out, strings.Join(tabs, ""), sDim.Render(strings.Repeat("─", W)))

	bodyH := H - len(out) - 1
	var body []string
	switch v.tab {
	case 0:
		body = v.secretsTab(W, bodyH)
	case 1:
		body = v.commandsTab(W, bodyH)
	case 2:
		body = v.accessTab(W)
	case 3:
		body = v.activityTab(W)
	}
	for i := 0; i < bodyH; i++ {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		out = append(out, fit(l, W))
	}

	foot := ""
	switch {
	case v.editing != "":
		foot = " " + v.input.View() + "   " + keyhelp("enter", "write Change", "esc", "cancel")
	case v.flash != "":
		foot = sWarn.Render(" " + v.flash)
	default:
		ks := []string{"1-4/tab", "tab", "[ ]", "Config", "s", "sync"}
		switch v.tab {
		case 0:
			if v.inCand {
				ks = []string{"j/k", "candidate", "enter", "resolve", "esc", "back"}
			} else {
				ks = append(ks, "enter", "edit/resolve", "v", "reveal", "a", "add", "d", "unset")
			}
		case 1:
			ks = append(ks, "enter", "run", "x", "close output")
		case 2:
			ks = append(ks, "hjkl", "cell", "space", "grant/revoke", "y", "approve join")
		case 3:
			ks = append(ks, "enter", "act on item")
		}
		ks = append(ks, "</>", "variant", "q", "quit")
		foot = " " + keyhelp(ks...)
	}
	out = append(out, foot)
	return strings.Join(out, "\n")
}

func (v *variantC) secretsTab(W, H int) []string {
	p, c := v.p(), v.c()
	if !c.Granted {
		return []string{"", sDim.Render("  No Grant on " + p.Name + "/" + c.Name + " for this Machine."),
			sDim.Render("  Ask an Admin (" + strings.Join(p.Admins, ", ") + ") — or run `kith request " + c.Name + "`.")}
	}
	lw := max(30, W*2/5)
	var left []string
	for i, s := range c.Secrets {
		flag := " "
		switch {
		case len(s.Conflict) > 0:
			flag = sWarn.Render("⚠")
		case s.NeedsRotation:
			flag = sErr.Render("⟳")
		case s.Inherited:
			flag = sDim.Render("↳")
		case s.Overridden:
			flag = sAccent.Render("●")
		case s.Ref != "":
			flag = sInfo.Render("$")
		}
		l := fmt.Sprintf(" %s %s", flag, s.Key)
		if i == v.sec {
			l = sSel.Render(fit(l, lw-1))
		}
		left = append(left, l)
	}
	if miss := p.Missing(c); len(miss) > 0 && p.Setup != "" {
		left = append(left, "", sWarn.Render(" requires missing:"))
		for _, k := range miss {
			left = append(left, sWarn.Render("   ✗ "+k))
		}
	}
	left = append(left, "", sDim.Render(" ⚠ Conflict ⟳ rotate ↳ inherited"), sDim.Render(" ● override  $ reference"))

	var right []string
	if s := v.s(); s != nil {
		val := mask(s.Value)
		if v.reveal {
			val = s.Value
		}
		right = append(right, sBold.Render(s.Key), "")
		right = append(right, sDim.Render("value   ")+val)
		switch {
		case s.Ref != "":
			right = append(right, sDim.Render("source  ")+sInfo.Render(s.Ref)+sDim.Render("  (resolved at run)"))
		case s.Inherited:
			right = append(right, sDim.Render("source  inherited from ")+c.Root+sDim.Render(" · e to override here"))
		case s.Overridden:
			right = append(right, sDim.Render("source  ")+sAccent.Render("overrides "+c.Root)+sDim.Render(" · d to go back to inheriting"))
		default:
			right = append(right, sDim.Render("source  "+c.Name))
		}
		if s.NeedsRotation {
			right = append(right, "", sErr.Render("⟳ A revoked Machine could read this value. Rotate it at the provider, then e."))
		}
		if len(s.Conflict) > 0 {
			right = append(right, "", sWarn.Render("⚠ Conflict: two concurrent Changes. `run` uses the first until resolved."))
			for i, cd := range s.Conflict {
				l := fmt.Sprintf("  %-22s %-12s %s", cd.Value, cd.Machine, sDim.Render(cd.When))
				if v.inCand && i == v.cand {
					l = sSel.Render("▸" + l[1:])
				}
				right = append(right, l)
			}
			nl := "  ✎ new value…"
			if v.inCand && v.cand == len(s.Conflict) {
				nl = sSel.Render("▸ ✎ new value…")
			}
			right = append(right, nl)
			if !v.inCand {
				right = append(right, sDim.Render("  enter to choose"))
			}
		}
		if len(s.History) > 0 {
			right = append(right, "", sDim.Render("history"))
			for _, h := range s.History {
				right = append(right, sDim.Render("  "+h))
			}
		}
	}
	var out []string
	for i := 0; i < H; i++ {
		a, b := "", ""
		if i < len(left) {
			a = left[i]
		}
		if i < len(right) {
			b = right[i]
		}
		out = append(out, fit(a, lw)+sDim.Render("│ ")+b)
	}
	return out
}

func (v *variantC) commandsTab(W, H int) []string {
	p, c := v.p(), v.c()
	if len(p.Commands) == 0 {
		return []string{"", sDim.Render("  " + p.Name + " has no kith.yaml in this repo.")}
	}
	var out []string
	miss := p.Missing(c)
	req := " requires "
	for _, k := range p.Requires {
		if c.Find(k) != nil {
			req += sOK.Render("✓"+k) + " "
		} else {
			req += sErr.Render("✗"+k) + " "
		}
	}
	out = append(out, req, "")
	for i, cmd := range p.Commands {
		state := sOK.Render("ready")
		switch {
		case !cmd.Allowed(c):
			state = sDim.Render(fmt.Sprintf("only %v", cmd.Envs))
		case len(miss) > 0:
			state = sErr.Render("missing keys")
		}
		l := fmt.Sprintf(" %-12s %-14s %s", cmd.Name, fit(state, 14), sDim.Render(cmd.Run))
		if i == v.cmd {
			l = sSel.Render(fmt.Sprintf(" %-12s ", cmd.Name)) + fmt.Sprintf("%-14s %s", fit(state, 14), sDim.Render(cmd.Run))
		}
		out = append(out, l)
		if i == v.cmd && cmd.Desc != "" {
			out = append(out, sDim.Render("   "+cmd.Desc))
		}
	}
	if v.out != nil {
		out = append(out, "", sDim.Render(strings.Repeat("─", W)))
		status := sOK.Render("● running")
		if v.out.done {
			status = sDim.Render("exited")
		}
		out = append(out, " "+sBold.Render(v.out.label)+"  "+status+sDim.Render("   (output pane · x closes · real kith would use a PTY)"))
		room := H - len(out)
		ls := v.out.lines
		if len(ls) > room {
			ls = ls[len(ls)-room:]
		}
		for _, l := range ls {
			out = append(out, " "+l)
		}
	}
	return out
}

func (v *variantC) accessTab(W int) []string {
	p := v.p()
	var out []string
	for _, m := range v.st.Machines {
		if m.JoinRequest {
			out = append(out, sWarn.Render(fmt.Sprintf(" ? %s asks to join (Member %s, fingerprint %s, tailnet %s) · y approve", m.Name, m.Member, m.Fingerprint, m.Addr)))
		}
	}
	if len(out) > 0 {
		out = append(out, "")
	}
	head := fmt.Sprintf(" %-8s %-16s %-22s", "MEMBER", "MACHINE", "STATUS")
	for _, c := range p.Configs {
		head += fmt.Sprintf(" %-10s", c.Name)
	}
	out = append(out, sDim.Render(head))
	last := ""
	for ri, m := range v.accessRows() {
		mem := m.Member
		if mem == last {
			mem = ""
		} else {
			for _, a := range p.Admins {
				if a == m.Member {
					mem += sAccent.Render("★")
				}
			}
		}
		last = m.Member
		_, status := machineStatus(m)
		l := fmt.Sprintf(" %s %-16s %s", fit(mem, 8), m.Name, fit(status, 22))
		for ci, c := range p.Configs {
			cell := sDim.Render("·")
			if m.Has(p.Name, c.Name) {
				cell = sOK.Render("●")
			}
			if m.Revoked {
				cell = sErr.Render("✖")
			}
			cell = " " + fit(cell, 10)
			if ri == v.row && ci == v.col {
				cell = " " + sSel.Render(fit(stripStyle(cell), 10))
			}
			l += cell
		}
		out = append(out, l)
	}
	out = append(out, "", sDim.Render(" ● Grant (read+write, Config encrypted to that Machine)   · none   ✖ revoked   ★ Admin"))
	for _, c := range p.Configs {
		if c.PendingRotation != "" {
			out = append(out, sErr.Render(fmt.Sprintf(" ⟳ %s: data key rotation pending after revoking %s (happens on next sync by a Machine holding %s)", c.Name, c.PendingRotation, c.Name)))
		}
	}
	return out
}

func stripStyle(s string) string { return strings.TrimSpace(ansiStrip(s)) }

func (v *variantC) activityTab(W int) []string {
	items := v.needsYou()
	out := []string{sBold.Render(" Needs you")}
	if len(items) == 0 {
		out = append(out, sDim.Render("   nothing 🎉"))
	}
	for i, it := range items {
		l := "  " + it.text
		if i == v.act {
			l = sSel.Render(" ▸") + it.text
		}
		out = append(out, l)
	}
	out = append(out, "", sBold.Render(" Feed"))
	for i := len(v.st.Log) - 1; i >= 0; i-- {
		out = append(out, "  "+colorLog(v.st.Log[i]))
	}
	return out
}
