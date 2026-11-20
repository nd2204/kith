package main

// PROTOTYPE — Variant B · Drill-down (k9s-style).
// One full-width list at a time. Enter drills in, Esc goes back, the breadcrumb shows
// where you are. `:` jumps anywhere (:machines :conflicts :commands :log :use stg :sync).
// An alert strip under the header lists everything that needs a human.
// Named commands hand the whole terminal to the process (tea.ExecProcess) and come back.

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type bKind int

const (
	bProjects bKind = iota
	bConfigs
	bSecrets
	bSecret
	bMachines
	bMachine
	bConflicts
	bCommands
	bLog
)

type bFrame struct {
	kind bKind
	p    *Project
	c    *Config
	key  string
	m    *Machine
	cur  int
}

type bRow struct {
	text string
	open func() *bFrame
}

type bExecDone struct{ name string }

type variantB struct {
	st     *Store
	stack  []*bFrame
	prompt string // "", "cmd", "edit", "add-key", "add-val", "resolve-new", "filter"
	input  textinput.Model
	pend   string
	filter string
	reveal bool
	flash  string
}

func newB(st *Store) *variantB {
	ti := textinput.New()
	p := st.Project("vet45-erp")
	v := &variantB{st: st, input: ti}
	v.stack = []*bFrame{{kind: bProjects}, {kind: bConfigs, p: p}, {kind: bSecrets, p: p, c: p.Config(st.Active[p.Name])}}
	v.stack[0].cur = 0
	return v
}

func (v *variantB) Key() string     { return "B" }
func (v *variantB) Name() string    { return "Drill-down (k9s)" }
func (v *variantB) Capturing() bool { return v.prompt != "" }
func (v *variantB) top() *bFrame    { return v.stack[len(v.stack)-1] }
func (v *variantB) push(f *bFrame) {
	if f != nil {
		v.stack = append(v.stack, f)
		v.filter = ""
	}
}

func (v *variantB) rows(f *bFrame) (header string, rows []bRow) {
	st := v.st
	switch f.kind {
	case bProjects:
		header = fmt.Sprintf("%-16s %-24s %-8s %s", "PROJECT", "SETUP", "CONFIGS", "ACTIVE")
		for _, p := range st.Projects {
			p := p
			rows = append(rows, bRow{fmt.Sprintf("%-16s %-24s %-8d %s", p.Name, p.Setup, len(p.Configs), st.Active[p.Name]),
				func() *bFrame { return &bFrame{kind: bConfigs, p: p} }})
		}
	case bConfigs:
		header = fmt.Sprintf("%-14s %-6s %-8s %-6s %s", "CONFIG", "ENV", "ROOT", "KEYS", "STATUS")
		for _, c := range f.p.Configs {
			c := c
			root := c.Root
			if root == "" {
				root = "-"
			}
			keys := fmt.Sprint(len(c.Secrets))
			if !c.Granted {
				keys = "-"
			}
			rows = append(rows, bRow{fmt.Sprintf("%-14s %-6s %-8s %-6s %s", c.Name, c.Env, root, keys, cfgBadges(c, st.Active[f.p.Name] == c.Name)),
				func() *bFrame { return &bFrame{kind: bSecrets, p: f.p, c: c} }})
		}
	case bSecrets:
		header = fmt.Sprintf("%-22s %-34s %s", "KEY", "VALUE", "SOURCE")
		if !f.c.Granted {
			return header, nil
		}
		for _, s := range f.c.Secrets {
			s := s
			if v.filter != "" && !strings.Contains(strings.ToLower(s.Key), strings.ToLower(v.filter)) {
				continue
			}
			val := mask(s.Value)
			if v.reveal {
				val = s.Value
			}
			src := ""
			switch {
			case s.Ref != "":
				src = sInfo.Render(s.Ref)
			case s.Inherited:
				src = sDim.Render("↳ " + f.c.Root)
			case s.Overridden:
				src = sAccent.Render("override")
			}
			if len(s.Conflict) > 0 {
				src += sWarn.Render(" ⚠ Conflict")
			}
			if s.NeedsRotation {
				src += sErr.Render(" ⟳ rotate")
			}
			rows = append(rows, bRow{fmt.Sprintf("%-22s %-34s %s", s.Key, fit(val, 34), src),
				func() *bFrame { return &bFrame{kind: bSecret, p: f.p, c: f.c, key: s.Key} }})
		}
	case bSecret:
		s := f.c.Find(f.key)
		if s == nil {
			return "", nil
		}
		if len(s.Conflict) > 0 {
			header = "CHOOSE THE VALUE THAT WINS"
			for _, cd := range s.Conflict {
				cd := cd
				rows = append(rows, bRow{fmt.Sprintf("%-24s from %-12s %s", cd.Value, cd.Machine, sDim.Render(cd.When)), func() *bFrame {
					v.st.Resolve(f.p, f.c, s.Key, cd.Value, "kept "+cd.Machine+"'s value")
					return nil
				}})
			}
			rows = append(rows, bRow{"✎ enter a new value…", func() *bFrame {
				v.prompt, v.pend = "resolve-new", s.Key
				v.input.SetValue("")
				v.input.Focus()
				return nil
			}})
		} else {
			header = "HISTORY (Change log of this key)"
			for _, h := range s.History {
				rows = append(rows, bRow{h, nil})
			}
		}
	case bMachines:
		header = fmt.Sprintf("  %-14s %-8s %-12s %-14s %s", "MACHINE", "MEMBER", "FINGERPRINT", "TAILNET", "STATUS")
		for _, m := range st.Machines {
			m := m
			dot, status := machineStatus(m)
			rows = append(rows, bRow{fmt.Sprintf("%s %-14s %-8s %-12s %-14s %s", dot, m.Name, m.Member, m.Fingerprint, m.Addr, status),
				func() *bFrame {
					if m.JoinRequest || m.Revoked {
						return nil
					}
					return &bFrame{kind: bMachine, m: m}
				}})
		}
	case bMachine:
		header = fmt.Sprintf("%-16s %-12s %-6s %s", "PROJECT", "CONFIG", "GRANT", "")
		for _, p := range st.Projects {
			for _, c := range p.Configs {
				p, c := p, c
				mark := sDim.Render("[ ]")
				if f.m.Has(p.Name, c.Name) {
					mark = sOK.Render("[x]")
				}
				note := ""
				if c.PendingRotation != "" {
					note = sErr.Render("⟳ rotation pending (" + c.PendingRotation + ")")
				}
				rows = append(rows, bRow{fmt.Sprintf("%-16s %-12s %s    %s", p.Name, c.Name, mark, note), func() *bFrame {
					v.st.ToggleGrant(f.m, p, c)
					return nil
				}})
			}
		}
	case bConflicts:
		header = fmt.Sprintf("%-16s %-10s %-20s %s", "PROJECT", "CONFIG", "KEY", "CANDIDATES")
		for _, p := range st.Projects {
			for _, c := range p.Configs {
				for _, s := range c.Secrets {
					if len(s.Conflict) == 0 {
						continue
					}
					p, c, s := p, c, s
					var who []string
					for _, cd := range s.Conflict {
						who = append(who, cd.Machine)
					}
					rows = append(rows, bRow{fmt.Sprintf("%-16s %-10s %-20s %s", p.Name, c.Name, s.Key, strings.Join(who, " vs ")),
						func() *bFrame { return &bFrame{kind: bSecret, p: p, c: c, key: s.Key} }})
				}
			}
		}
	case bCommands:
		header = fmt.Sprintf("%-12s %-12s %s", "COMMAND", "ENVS", "RUN")
		for _, cmd := range f.p.Commands {
			cmd := cmd
			envs := "any"
			if len(cmd.Envs) > 0 {
				envs = strings.Join(cmd.Envs, ",")
			}
			rows = append(rows, bRow{fmt.Sprintf("%-12s %-12s %s", cmd.Name, envs, sDim.Render(cmd.Run)), nil})
		}
	case bLog:
		header = "LOG"
		for i := len(st.Log) - 1; i >= 0; i-- {
			rows = append(rows, bRow{colorLog(st.Log[i]), nil})
		}
	}
	return
}

func (v *variantB) crumb() string {
	var parts []string
	for _, f := range v.stack {
		switch f.kind {
		case bProjects:
			parts = append(parts, "projects")
		case bConfigs:
			parts = append(parts, f.p.Name)
		case bSecrets:
			parts = append(parts, f.c.Name)
		case bSecret:
			parts = append(parts, f.key)
		case bMachines:
			parts = append(parts, "machines")
		case bMachine:
			parts = append(parts, f.m.Name)
		case bConflicts:
			parts = append(parts, "conflicts")
		case bCommands:
			parts = append(parts, f.p.Name+" commands")
		case bLog:
			parts = append(parts, "log")
		}
	}
	return strings.Join(parts, sDim.Render(" ▸ "))
}

func (v *variantB) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case bExecDone:
		v.st.logf("Run     kith run %s exited, back in kith", m.name)
		return nil
	case tea.KeyMsg:
		return v.key(m)
	}
	return nil
}

func (v *variantB) curProject() *Project {
	for i := len(v.stack) - 1; i >= 0; i-- {
		if v.stack[i].p != nil {
			return v.stack[i].p
		}
	}
	return v.st.Project("vet45-erp")
}

func (v *variantB) key(m tea.KeyMsg) tea.Cmd {
	k := m.String()
	v.flash = ""
	f := v.top()

	if v.prompt != "" {
		switch k {
		case "esc":
			v.prompt = ""
			v.input.Blur()
			return nil
		case "enter":
			val := strings.TrimSpace(v.input.Value())
			kind := v.prompt
			v.prompt = ""
			v.input.Blur()
			switch kind {
			case "cmd":
				v.command(val)
			case "filter":
				v.filter = val
			case "edit":
				v.st.Set(f.p, f.c, v.pend, val)
			case "add-key":
				v.pend = strings.ToUpper(val)
				v.prompt = "add-val"
				v.input.SetValue("")
				return v.input.Focus()
			case "add-val":
				v.st.Set(f.p, f.c, v.pend, val)
			case "resolve-new":
				v.st.Resolve(f.p, f.c, v.pend, val, "new value")
				v.stack = v.stack[:len(v.stack)-1]
			}
			return nil
		}
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(m)
		if v.prompt == "filter" {
			v.filter = v.input.Value()
		}
		return cmd
	}

	_, rows := v.rows(f)
	switch k {
	case "j", "down":
		f.cur = move(f.cur, len(rows), 1)
	case "k", "up":
		f.cur = move(f.cur, len(rows), -1)
	case "esc", "h":
		if v.filter != "" {
			v.filter = ""
		} else if len(v.stack) > 1 {
			v.stack = v.stack[:len(v.stack)-1]
		}
	case "enter", "l", " ", "space":
		if f.kind == bCommands && len(f.p.Commands) > 0 {
			return v.run(f.p, f.p.Commands[f.cur])
		}
		if f.cur < len(rows) && rows[f.cur].open != nil {
			v.push(rows[f.cur].open())
			if f.kind == bSecret && v.prompt == "" && len(v.stack) > 1 {
				v.stack = v.stack[:len(v.stack)-1] // resolved → back to the list
			}
		}
	case ":":
		v.prompt = "cmd"
		v.input.Prompt = ":"
		v.input.SetValue("")
		return v.input.Focus()
	case "/":
		v.prompt = "filter"
		v.input.Prompt = "/"
		v.input.SetValue("")
		return v.input.Focus()
	case "v":
		v.reveal = !v.reveal
	case "s":
		v.st.Sync()
	case "u":
		if f.kind == bConfigs && f.cur < len(f.p.Configs) {
			v.st.Use(f.p, f.p.Configs[f.cur])
		}
	case "R":
		if f.c != nil && f.c.Dropped > 0 {
			v.st.Reapply(f.p, f.c)
		}
	case "y":
		if f.kind == bMachines && f.cur < len(v.st.Machines) && v.st.Machines[f.cur].JoinRequest {
			v.st.Approve(v.st.Machines[f.cur])
		}
	case "e":
		if (f.kind == bSecrets || f.kind == bSecret) && f.c.Granted {
			key := f.key
			if f.kind == bSecrets && f.cur < len(f.c.Secrets) {
				key = f.c.Secrets[f.cur].Key
			}
			v.prompt, v.pend = "edit", key
			v.input.Prompt = "edit " + key + " = "
			v.input.SetValue(f.c.Find(key).Value)
			return v.input.Focus()
		}
	case "a":
		if f.kind == bSecrets && f.c.Granted {
			v.prompt = "add-key"
			v.input.Prompt = "new key: "
			v.input.SetValue("")
			return v.input.Focus()
		}
	case "d":
		if f.kind == bSecrets && f.cur < len(f.c.Secrets) {
			v.st.Unset(f.p, f.c, f.c.Secrets[f.cur].Key)
		}
	}
	return nil
}

func (v *variantB) command(s string) {
	p := v.curProject()
	verb, arg, _ := strings.Cut(s, " ")
	reset := func(fr ...*bFrame) { v.stack = append([]*bFrame{{kind: bProjects}}, fr...) }
	switch verb {
	case "machines", "mach", "m":
		reset(&bFrame{kind: bMachines})
	case "conflicts", "conf":
		reset(&bFrame{kind: bConflicts})
	case "commands", "cmd", "run":
		reset(&bFrame{kind: bConfigs, p: p}, &bFrame{kind: bCommands, p: p})
	case "log":
		reset(&bFrame{kind: bLog})
	case "projects", "p":
		reset()
	case "secrets", "configs":
		reset(&bFrame{kind: bConfigs, p: p})
	case "use":
		if c := p.Config(arg); c != nil {
			v.st.Use(p, c)
			reset(&bFrame{kind: bConfigs, p: p}, &bFrame{kind: bSecrets, p: p, c: c})
		} else {
			v.flash = "no Config " + arg + " in " + p.Name
		}
	case "sync":
		v.st.Sync()
	default:
		if pp := v.st.Project(verb); pp != nil {
			reset(&bFrame{kind: bConfigs, p: pp})
			return
		}
		v.flash = "unknown :" + s + "  (try :machines :conflicts :commands :log :use <config> :sync :<project>)"
	}
}

func (v *variantB) run(p *Project, cmd Command) tea.Cmd {
	c := p.Config(v.st.Active[p.Name])
	if !cmd.Allowed(c) {
		v.flash = fmt.Sprintf("%s only runs in %v; Active config is %s", cmd.Name, cmd.Envs, c.Name)
		return nil
	}
	if miss := p.Missing(c); len(miss) > 0 {
		v.flash = "missing required keys: " + strings.Join(miss, ", ")
		return nil
	}
	script := fmt.Sprintf(`clear; printf '\033[2m$ kith run %s   # %s/%s, %d keys injected\033[0m\n'; echo '+ %s'; echo '(prototype: pretending to run)'; sleep 1; printf '\nprocess exited 0. Press Enter to return to kith… '; read _`,
		cmd.Name, p.Name, c.Name, len(c.Secrets), strings.ReplaceAll(cmd.Run, "'", ""))
	return tea.ExecProcess(exec.Command("sh", "-c", script), func(error) tea.Msg { return bExecDone{cmd.Name} })
}

func (v *variantB) alerts() []string {
	var a []string
	for _, p := range v.st.Projects {
		for _, c := range p.Configs {
			if !c.Granted {
				continue
			}
			if n := c.Conflicts(); n > 0 {
				a = append(a, sWarn.Render(fmt.Sprintf("⚠ %d Conflict in %s/%s (:conflicts)", n, p.Name, c.Name)))
			}
			if c.PendingRotation != "" {
				a = append(a, sErr.Render(fmt.Sprintf("⟳ %s/%s key rotation pending after revoking %s; %d keys to rotate", p.Name, c.Name, c.PendingRotation, len(c.RotationKeys()))))
			}
			if c.Dropped > 0 {
				a = append(a, sErr.Render(fmt.Sprintf("✖ %d Changes dropped from %s on %s/%s (R in that Config)", c.Dropped, c.DroppedFrom, p.Name, c.Name)))
			}
		}
	}
	for _, m := range v.st.Machines {
		if m.JoinRequest {
			a = append(a, sWarn.Render(fmt.Sprintf("? %s wants to join (:machines, y)", m.Name)))
		}
	}
	return a
}

func (v *variantB) View(W, H int) string {
	f := v.top()
	this := v.st.This()
	on, total := v.st.Online()
	head := sAccent.Render(" kith ") + " " + v.crumb()
	right := sDim.Render(fmt.Sprintf("%s · peers %d/%d · synced %s ", this.Name, on, total, v.st.LastSync))
	gap := max(1, W-lenVisible(head)-lenVisible(right))
	out := []string{head + strings.Repeat(" ", gap) + right}

	al := v.alerts()
	for i, a := range al {
		if i >= 3 {
			out = append(out, sDim.Render(fmt.Sprintf("   +%d more", len(al)-3)))
			break
		}
		out = append(out, " "+a)
	}
	out = append(out, sDim.Render(strings.Repeat("─", W)))

	// detail header for a single secret
	if f.kind == bSecret {
		if s := f.c.Find(f.key); s != nil {
			val := mask(s.Value)
			if v.reveal {
				val = s.Value
			}
			out = append(out, " "+sBold.Render(s.Key)+sDim.Render("  in "+f.p.Name+"/"+f.c.Name), " value   "+val)
			if s.Ref != "" {
				out = append(out, " ref     "+sInfo.Render(s.Ref))
			}
			if len(s.Conflict) > 0 {
				out = append(out, sWarn.Render(" ⚠ Conflict · `run` uses "+s.Conflict[0].Machine+"'s value until resolved"))
			}
			out = append(out, "")
		}
	}
	if f.kind == bSecrets && !f.c.Granted {
		out = append(out, "", sDim.Render("  No Grant on "+f.p.Name+"/"+f.c.Name+". Ask an Admin ("+strings.Join(f.p.Admins, ", ")+")."))
	}
	if f.kind == bSecrets && f.c.Granted {
		if miss := f.p.Missing(f.c); len(miss) > 0 && f.p.Setup != "" {
			out = append(out, sWarn.Render(" kith.yaml requires missing: "+strings.Join(miss, ", ")))
		}
	}

	header, rows := v.rows(f)
	title := fmt.Sprintf(" %s [%d]", header, len(rows))
	if v.filter != "" {
		title += sInfo.Render("  /" + v.filter)
	}
	out = append(out, sDim.Render(title))
	listH := H - len(out) - 2
	from, to := window(len(rows), f.cur, listH)
	for i := from; i < to; i++ {
		l := " " + rows[i].text
		if i == f.cur {
			l = sSel.Render(fit(l, W))
		}
		out = append(out, l)
	}
	for len(out) < H-1 {
		out = append(out, "")
	}
	out = out[:H-1]

	foot := ""
	switch {
	case v.prompt != "":
		foot = " " + v.input.View()
	case v.flash != "":
		foot = sWarn.Render(" " + v.flash)
	default:
		ks := []string{"enter", "open", "esc", "back", ":", "jump", "/", "filter", "v", "reveal", "s", "sync"}
		switch f.kind {
		case bSecrets:
			ks = append(ks, "e", "edit", "a", "add", "d", "unset", "R", "re-apply dropped")
		case bSecret:
			ks = append(ks, "e", "edit")
		case bConfigs:
			ks = append(ks, "u", "kith use")
		case bMachines:
			ks = append(ks, "y", "approve join")
		case bMachine:
			ks = append(ks[:2], "space", "grant/revoke", "esc", "back")
		case bCommands:
			ks = []string{"enter", "run (hands over the terminal)", "esc", "back"}
		}
		ks = append(ks, "</>", "variant")
		foot = " " + keyhelp(ks...)
	}
	out = append(out, foot)
	return strings.Join(out, "\n")
}

func lenVisible(s string) int { return ansi.StringWidth(s) }
