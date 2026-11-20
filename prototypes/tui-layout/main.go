// PROTOTYPE — throwaway, do not merge.
//
// Question (nd2204/kith#9 "Bố cục TUI Bubble Tea của kith"): what should the kith TUI
// look like? Three structurally different layouts over the same mock state,
// switchable with `<` / `>` (or `--variant=A|B|C`):
//
//	A · Panels      lazygit-style: stacked side panels + main panel + log, commands run in embedded sessions
//	B · Drill-down  k9s-style: one full-width list, Enter drills in, `:` jumps, commands hand the terminal over
//	C · Setup tabs  starts from the cwd's kith.yaml: Config chips + tabs (Secrets/Commands/Access/Activity)
//
// All state is in memory (data.go). Nothing is encrypted, nothing syncs; `s` fakes a sync.
//
//	go run .                 # interactive
//	go run . --variant=C
//	go run . --dump          # print scripted screens of every variant (no TTY needed)
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ---------- shared look ----------

var (
	cAccent = lipgloss.Color("#7aa2f7")
	cDim    = lipgloss.Color("#6b7089")
	cWarn   = lipgloss.Color("#e0af68")
	cErr    = lipgloss.Color("#f7768e")
	cOK     = lipgloss.Color("#9ece6a")
	cInfo   = lipgloss.Color("#7dcfff")

	sAccent = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sWarn   = lipgloss.NewStyle().Foreground(cWarn)
	sErr    = lipgloss.NewStyle().Foreground(cErr)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sInfo   = lipgloss.NewStyle().Foreground(cInfo)
	sBold   = lipgloss.NewStyle().Bold(true)
	sSel    = lipgloss.NewStyle().Background(lipgloss.Color("#2f3549")).Bold(true)
	sKey    = lipgloss.NewStyle().Foreground(cAccent)
)

// fit truncates or pads s (which may contain ANSI) to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// box draws a rounded panel of exactly w×h cells with the title set in the top border.
func box(title string, lines []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	bc := sDim
	if focused {
		bc = lipgloss.NewStyle().Foreground(cAccent)
	}
	t := ""
	if title != "" {
		t = " " + title + " "
	}
	top := bc.Render("╭─") + t
	top += bc.Render(strings.Repeat("─", max(0, w-3-ansi.StringWidth(t))) + "╮")
	top = fit(top, w)
	out := []string{top}
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, bc.Render("│")+fit(l, w-2)+bc.Render("│"))
	}
	out = append(out, bc.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return strings.Join(out, "\n")
}

// pad makes a block exactly h lines of width w.
func pad(block string, w, h int) string {
	ls := strings.Split(block, "\n")
	out := make([]string, h)
	for i := range out {
		if i < len(ls) {
			out[i] = fit(ls[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return strings.Join(out, "\n")
}

func keyhelp(pairs ...string) string {
	var b []string
	for i := 0; i+1 < len(pairs); i += 2 {
		b = append(b, sKey.Render(pairs[i])+" "+sDim.Render(pairs[i+1]))
	}
	return strings.Join(b, "  ")
}

// scroll returns the window of n lines around cursor.
func window(total, cursor, n int) (from, to int) {
	if total <= n {
		return 0, total
	}
	from = cursor - n/2
	from = max(0, min(from, total-n))
	return from, from + n
}

// ---------- fake run sessions (shared by A and C) ----------

type session struct {
	id    int
	label string
	lines []string
	queue []string
	done  bool
	long  bool // keeps running after its scripted output (flutter run)
}

type sessionTick struct{ id int }

var sessionSeq int

func newSession(p *Project, c *Config, cmd Command) (*session, tea.Cmd) {
	sessionSeq++
	n := len(c.Secrets)
	q := []string{
		sDim.Render(fmt.Sprintf("$ kith run %s   # %s/%s, %d keys injected + KITH_PROJECT/KITH_CONFIG/KITH_ENVIRONMENT", cmd.Name, p.Name, c.Name, n)),
	}
	for _, s := range c.Secrets {
		if len(s.Conflict) > 0 {
			q = append(q, sWarn.Render(fmt.Sprintf("kith: warning: Conflict on %s, using %s's value (resolve with `kith secrets resolve %s`)", s.Key, s.Conflict[0].Machine, s.Key)))
		}
	}
	q = append(q, "+ "+cmd.Run)
	switch {
	case strings.HasPrefix(cmd.Run, "flutter run"):
		q = append(q, "Launching lib/main.dart on iPhone 16 Pro in debug mode...", "Running Xcode build...", "Xcode build done.                                            18.4s",
			"Syncing files to device iPhone 16 Pro...", "Flutter run key commands.", "r Hot reload. 🔥🔥🔥", "flutter: APP_ENV="+valueOf(c, "APP_ENV"))
	case strings.HasPrefix(cmd.Run, "flutter test"):
		q = append(q, "00:01 +0: loading test/widget_test.dart", "00:03 +12: All tests passed!", sOK.Render("exit 0"))
	default:
		q = append(q, "working…", "done", sOK.Render("exit 0"))
	}
	s := &session{id: sessionSeq, label: cmd.Name + "@" + c.Name, queue: q, long: strings.HasPrefix(cmd.Run, "flutter run")}
	return s, tickSession(s.id)
}

func valueOf(c *Config, k string) string {
	if s := c.Find(k); s != nil {
		return s.Value
	}
	return ""
}

func tickSession(id int) tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return sessionTick{id} })
}

func (s *session) step() tea.Cmd {
	if len(s.queue) == 0 {
		return nil
	}
	s.lines = append(s.lines, s.queue[0])
	s.queue = s.queue[1:]
	if len(s.queue) == 0 {
		s.done = !s.long
		return nil
	}
	return tickSession(s.id)
}

// ---------- root model + variant switcher ----------

type Variant interface {
	Key() string
	Name() string
	Update(tea.Msg) tea.Cmd
	View(w, h int) string
	Capturing() bool // a text input is focused: don't steal `<`, `>`, `q`
}

type root struct {
	st       *Store
	variants []Variant
	cur      int
	w, h     int
}

func (r *root) Init() tea.Cmd { return nil }

func (r *root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		r.w, r.h = m.Width, m.Height
		return r, nil
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return r, tea.Quit
		}
		v := r.variants[r.cur]
		if !v.Capturing() {
			switch m.String() {
			case "q":
				return r, tea.Quit
			case "<", "ctrl+left":
				r.cur = (r.cur + len(r.variants) - 1) % len(r.variants)
				return r, nil
			case ">", "ctrl+right":
				r.cur = (r.cur + 1) % len(r.variants)
				return r, nil
			}
		}
		return r, v.Update(msg)
	}
	// non-key messages (session ticks, exec results) go to every variant
	var cmds []tea.Cmd
	for _, v := range r.variants {
		cmds = append(cmds, v.Update(msg))
	}
	return r, tea.Batch(cmds...)
}

func (r *root) View() string {
	if r.w == 0 {
		return "…"
	}
	v := r.variants[r.cur]
	body := pad(v.View(r.w, r.h-1), r.w, r.h-1)
	return body + "\n" + r.switcher()
}

func (r *root) switcher() string {
	v := r.variants[r.cur]
	pill := lipgloss.NewStyle().Background(lipgloss.Color("#bb9af7")).Foreground(lipgloss.Color("#1a1b26")).Bold(true)
	bar := lipgloss.NewStyle().Background(lipgloss.Color("#3b3052")).Foreground(lipgloss.Color("#e0def4"))
	on, total := r.st.Online()
	state := fmt.Sprintf(" local Changes %d · Conflicts %d · peers %d/%d · sync %s · log %d ",
		r.st.Local, r.st.AllConflicts(), on, total, r.st.LastSync, len(r.st.Log))
	left := pill.Render(fmt.Sprintf(" PROTOTYPE  ◀ <  %s · %s  > ▶ ", v.Key(), v.Name()))
	return left + bar.Render(fit(state, max(0, r.w-ansi.StringWidth(left))))
}

func main() {
	variant := flag.String("variant", "A", "A, B or C")
	dump := flag.Bool("dump", false, "print scripted screens of every variant and exit")
	w := flag.Int("w", 120, "dump width")
	h := flag.Int("h", 34, "dump height")
	flag.Parse()

	st := NewStore()
	r := &root{st: st, variants: []Variant{newA(st), newB(st), newC(st)}}
	for i, v := range r.variants {
		if strings.EqualFold(v.Key(), *variant) {
			r.cur = i
		}
	}
	if *dump {
		runDump(*w, *h)
		return
	}
	if _, err := tea.NewProgram(r, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ---------- dump: scripted screens for review without a TTY ----------

func press(r *root, keys ...string) {
	for _, k := range keys {
		var m tea.KeyMsg
		switch k {
		case "tab":
			m = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			m = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "enter":
			m = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			m = tea.KeyMsg{Type: tea.KeyEsc}
		case "up":
			m = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			m = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			m = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			m = tea.KeyMsg{Type: tea.KeyRight}
		case "space":
			m = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "bs":
			m = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			m = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		r.Update(m)
	}
}

func flushTicks(r *root) {
	for i := 0; i < 20; i++ {
		for id := 1; id <= sessionSeq; id++ {
			r.Update(sessionTick{id})
		}
	}
}

func runDump(w, h int) {
	type scene struct {
		variant, title string
		keys           []string
	}
	scenes := []scene{
		{"A", "start", nil},
		{"A", "switcher: > goes to B", []string{">"}},
		{"A", "Conflict on STRIPE_KEY (j×3, enter)", []string{"j", "j", "j", "enter"}},
		{"A", "Configs panel on stg (dropped Changes banner)", []string{"2", "j", "j"}},
		{"A", "Machines panel → grant matrix for bob-mbp", []string{"4", "j", "j"}},
		{"A", "Commands panel → run `debug` in an embedded session", []string{"3", "enter"}},
		{"B", "start (Setup of cwd: vet45-erp ▸ dev)", nil},
		{"B", ":machines", []string{":", "m", "a", "c", "h", "i", "n", "e", "s", "enter"}},
		{"B", "machine bob-mbp → Grants", []string{":", "m", "a", "c", "h", "i", "n", "e", "s", "enter", "j", "j", "enter"}},
		{"B", "Conflict screen", []string{":", "c", "o", "n", "f", "l", "i", "c", "t", "s", "enter", "enter"}},
		{"B", "Conflict → new value prompt", []string{":", "c", "o", "n", "f", "l", "i", "c", "t", "s", "enter", "enter", "j", "j", "enter", "x", "enter"}},
		{"C", "Secrets tab", nil},
		{"C", "Secrets tab, STRIPE_KEY selected", []string{"j", "j", "j"}},
		{"C", "Commands tab", []string{"2", "enter"}},
		{"C", "Access tab", []string{"3"}},
		{"C", "Activity tab", []string{"4"}},
	}
	for _, sc := range scenes {
		st := NewStore()
		sessionSeq = 0
		r := &root{st: st, variants: []Variant{newA(st), newB(st), newC(st)}, w: w, h: h}
		for i, v := range r.variants {
			if v.Key() == sc.variant {
				r.cur = i
			}
		}
		press(r, sc.keys...)
		flushTicks(r)
		fmt.Printf("\n===== %s · %s =====\n", sc.variant, sc.title)
		fmt.Println(r.View())
	}
}

func ansiStrip(s string) string { return ansi.Strip(s) }
