package main

// PROTOTYPE — throwaway. Mock, in-memory kith state shared by every variant so an
// edit made in one variant is visible after switching to another.

import (
	"fmt"
	"sort"
	"strings"
)

type Candidate struct {
	Value   string
	Machine string
	When    string
}

type Secret struct {
	Key           string
	Value         string // resolved value (what `run` would inject)
	Ref           string // raw `${project.config.KEY}` when the value is a reference
	Inherited     bool   // Branch config: comes from root, not overridden
	Overridden    bool   // Branch config: overrides root
	NeedsRotation bool   // value was readable by a revoked Machine
	Conflict      []Candidate
	History       []string
}

type Config struct {
	Name            string
	Env             string
	Root            string // non-empty for a Branch config
	Granted         bool   // this Machine has a Grant (holds the data key)
	Secrets         []*Secret
	PendingRotation string // revoked Machine whose data-key rotation is still waiting
	Dropped         int    // Changes dropped from a revoked Machine
	DroppedFrom     string
}

func (c *Config) Conflicts() int {
	n := 0
	for _, s := range c.Secrets {
		if len(s.Conflict) > 0 {
			n++
		}
	}
	return n
}

func (c *Config) RotationKeys() []string {
	var ks []string
	for _, s := range c.Secrets {
		if s.NeedsRotation {
			ks = append(ks, s.Key)
		}
	}
	return ks
}

func (c *Config) Find(key string) *Secret {
	for _, s := range c.Secrets {
		if s.Key == key {
			return s
		}
	}
	return nil
}

type Command struct {
	Name, Run, Desc, Dir string
	Envs                 []string
}

type Project struct {
	Name     string
	Setup    string // path of kith.yaml, empty if no Setup in this repo
	Default  string // config in Setup
	Requires []string
	Configs  []*Config
	Commands []Command
	Admins   []string // Members
}

func (p *Project) Config(name string) *Config {
	for _, c := range p.Configs {
		if c.Name == name {
			return c
		}
	}
	return nil
}

type Machine struct {
	Name        string
	Member      string
	Fingerprint string
	Addr        string
	This        bool
	Online      bool
	Behind      int // Changes this peer has not seen yet
	LastSeen    string
	Revoked     bool
	JoinRequest bool                       // waiting for an Admin to approve `kith join`
	Grants      map[string]map[string]bool // project -> config -> granted
}

func (m *Machine) Has(p, c string) bool { return m.Grants[p] != nil && m.Grants[p][c] }

type Store struct {
	Projects []*Project
	Machines []*Machine
	Active   map[string]string // project -> Active config on this Machine
	Log      []string
	Local    int // local Changes not yet seen by any peer
	LastSync string
}

func (s *Store) Project(name string) *Project {
	for _, p := range s.Projects {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (s *Store) This() *Machine {
	for _, m := range s.Machines {
		if m.This {
			return m
		}
	}
	return nil
}

func (s *Store) logf(f string, a ...any) {
	s.Log = append(s.Log, fmt.Sprintf(f, a...))
}

func (s *Store) Online() (on, total int) {
	for _, m := range s.Machines {
		if m.This || m.Revoked || m.JoinRequest {
			continue
		}
		total++
		if m.Online {
			on++
		}
	}
	return
}

func (s *Store) AllConflicts() (n int) {
	for _, p := range s.Projects {
		for _, c := range p.Configs {
			if c.Granted {
				n += c.Conflicts()
			}
		}
	}
	return
}

// --- mutations (each one is "a signed Change written locally") ---

func (s *Store) Set(p *Project, c *Config, key, val string) {
	sec := c.Find(key)
	if sec == nil {
		sec = &Secret{Key: key}
		c.Secrets = append(c.Secrets, sec)
		sort.Slice(c.Secrets, func(i, j int) bool { return c.Secrets[i].Key < c.Secrets[j].Key })
	}
	sec.Value, sec.Ref = val, ""
	if strings.HasPrefix(val, "${") {
		sec.Ref = val
		sec.Value = "(resolved at run)"
	}
	sec.NeedsRotation = false
	if c.Root != "" {
		sec.Inherited, sec.Overridden = false, true
	}
	sec.History = append([]string{"just now · alice-mbp · set"}, sec.History...)
	s.Local++
	s.logf("Change  set %s on %s/%s (local, waiting for peers)", key, p.Name, c.Name)
}

func (s *Store) Unset(p *Project, c *Config, key string) {
	for i, sec := range c.Secrets {
		if sec.Key == key {
			c.Secrets = append(c.Secrets[:i], c.Secrets[i+1:]...)
			break
		}
	}
	s.Local++
	s.logf("Change  unset %s on %s/%s", key, p.Name, c.Name)
}

func (s *Store) Resolve(p *Project, c *Config, key, val, from string) {
	sec := c.Find(key)
	if sec == nil {
		return
	}
	sec.Value, sec.Conflict = val, nil
	sec.History = append([]string{"just now · alice-mbp · resolve (" + from + ")"}, sec.History...)
	s.Local++
	s.logf("Change  resolve %s on %s/%s → %s", key, p.Name, c.Name, from)
}

func (s *Store) ToggleGrant(m *Machine, p *Project, c *Config) {
	if m.Grants[p.Name] == nil {
		m.Grants[p.Name] = map[string]bool{}
	}
	if m.Grants[p.Name][c.Name] {
		m.Grants[p.Name][c.Name] = false
		c.PendingRotation = m.Name
		for _, sec := range c.Secrets {
			sec.NeedsRotation = true
		}
		s.logf("Project revoke Grant %s/%s from %s (signed by alice, Admin) · data key rotation pending", p.Name, c.Name, m.Name)
	} else {
		m.Grants[p.Name][c.Name] = true
		s.logf("Project grant %s/%s to %s (signed by alice, Admin) · a peer holding %s will wrap the data key", p.Name, c.Name, m.Name, c.Name)
	}
	s.Local++
}

func (s *Store) Approve(m *Machine) {
	m.JoinRequest = false
	m.Online = true
	m.Grants = map[string]map[string]bool{}
	s.Local++
	s.logf("Project approve join %s as Member %s (fingerprint %s) · no Grants yet", m.Name, m.Member, m.Fingerprint)
}

func (s *Store) Reapply(p *Project, c *Config) {
	s.logf("Change  re-applied %d dropped Changes from %s on %s/%s (now authored by alice-mbp)", c.Dropped, c.DroppedFrom, p.Name, c.Name)
	s.Local += c.Dropped
	c.Dropped = 0
}

func (s *Store) Use(p *Project, c *Config) {
	s.Active[p.Name] = c.Name
	s.logf("Local   kith use %s (Project %s, this Machine only)", c.Name, p.Name)
}

func (s *Store) Sync() {
	for _, m := range s.Machines {
		if m.Online {
			m.Behind = 0
		}
	}
	s.Local = 0
	s.LastSync = "just now"
	for _, p := range s.Projects {
		for _, c := range p.Configs {
			if c.PendingRotation != "" && c.Granted {
				s.logf("Sync    rotated data key of %s/%s (epoch +1) after revoking %s", p.Name, c.Name, c.PendingRotation)
				c.PendingRotation = ""
			}
		}
	}
	s.logf("Sync    exchanged heads with %d online peers", func() int { n, _ := s.Online(); return n }())
}

// --- seed ---

func mask(v string) string {
	if v == "" {
		return ""
	}
	return "••••••••"
}

func NewStore() *Store {
	sec := func(k, v string, hist ...string) *Secret { return &Secret{Key: k, Value: v, History: hist} }

	erpDev := &Config{Name: "dev", Env: "dev", Granted: true, Secrets: []*Secret{
		sec("APP_ENV", "development", "3d ago · bob-mbp · set"),
		sec("FLAVOR", "dev", "3w ago · alice-mbp · set"),
		{Key: "SENTRY_DSN", Ref: "${vet45-shared.dev.SENTRY_DSN}", Value: "https://abc@o1.ingest.sentry.io/42", History: []string{"2w ago · alice-mbp · set"}},
		{Key: "STRIPE_KEY", Value: "sk_test_51Hq…a9", History: []string{"5m ago · bob-mbp · set", "5m ago · alice-mini · set", "1mo ago · alice-mbp · set"},
			Conflict: []Candidate{{"sk_test_51Hq…a9", "bob-mbp", "5m ago"}, {"sk_test_77Zx…k2", "alice-mini", "6m ago"}}},
		sec("SUPABASE_ANON_KEY", "eyJhbGciOiJIUzI1NiIs…", "1mo ago · alice-mbp · set"),
		sec("SUPABASE_URL", "https://dev-xyz.supabase.co", "1mo ago · alice-mbp · set"),
	}}
	erpDevAlice := &Config{Name: "dev_alice", Env: "dev", Root: "dev", Granted: true, Secrets: []*Secret{
		{Key: "APP_ENV", Value: "development", Inherited: true},
		{Key: "FLAVOR", Value: "dev", Inherited: true},
		{Key: "SUPABASE_URL", Value: "http://127.0.0.1:54321", Overridden: true, History: []string{"1w ago · alice-mbp · set"}},
		{Key: "SUPABASE_ANON_KEY", Value: "eyJlocal…", Overridden: true, History: []string{"1w ago · alice-mbp · set"}},
	}}
	erpStg := &Config{Name: "stg", Env: "stg", Granted: true, Dropped: 2, DroppedFrom: "bob-old-laptop", Secrets: []*Secret{
		sec("APP_ENV", "staging"), sec("FLAVOR", "stg"),
		sec("SUPABASE_ANON_KEY", "eyJstg…"), sec("SUPABASE_URL", "https://stg-xyz.supabase.co"),
	}}
	erpPrd := &Config{Name: "prd", Env: "prd", Granted: true, PendingRotation: "bob-old-laptop", Secrets: []*Secret{
		sec("APP_ENV", "production"), sec("FLAVOR", "prd"),
		{Key: "STRIPE_KEY", Value: "sk_live_9a…77", NeedsRotation: true},
		{Key: "SUPABASE_ANON_KEY", Value: "eyJprd…", NeedsRotation: true},
		sec("SUPABASE_URL", "https://prd-xyz.supabase.co"),
	}}
	erp := &Project{
		Name: "vet45-erp", Setup: "apps/erp/kith.yaml", Default: "dev",
		Requires: []string{"SUPABASE_URL", "SUPABASE_ANON_KEY", "STRIPE_KEY"},
		Configs:  []*Config{erpDev, erpDevAlice, erpStg, erpPrd},
		Admins:   []string{"alice"},
		Commands: []Command{
			{Name: "debug", Run: `flutter run --flavor "$FLAVOR" --dart-define-from-file=$KITH_ENV_FILE`, Desc: "Flutter run với flavor đang chọn", Dir: "."},
			{Name: "test", Run: "flutter test", Dir: "."},
			{Name: "gen:l10n", Run: "flutter gen-l10n", Desc: "Sinh file l10n", Dir: "."},
			{Name: "deploy-ios", Run: "./deploy-ios.sh", Desc: "Build và đẩy TestFlight", Dir: ".", Envs: []string{"stg", "prd"}},
		},
	}
	shared := &Project{Name: "vet45-shared", Default: "dev", Admins: []string{"alice"}, Configs: []*Config{
		{Name: "dev", Env: "dev", Granted: true, Secrets: []*Secret{sec("SENTRY_DSN", "https://abc@o1.ingest.sentry.io/42")}},
		{Name: "prd", Env: "prd", Granted: false},
	}}
	mau := &Project{Name: "mau-hien-app", Setup: "apps/mau_hien/kith.yaml", Default: "dev", Admins: []string{"bob"},
		Requires: []string{"API_BASE_URL"},
		Configs: []*Config{
			{Name: "dev", Env: "dev", Granted: true, Secrets: []*Secret{sec("API_BASE_URL", "https://api.dev.mauhien.vn")}},
			{Name: "prd", Env: "prd", Granted: false},
		},
		Commands: []Command{{Name: "serve", Run: "npm run dev", Dir: "."}},
	}

	g := func(pairs ...string) map[string]map[string]bool {
		out := map[string]map[string]bool{}
		for _, pc := range pairs {
			p, c, _ := strings.Cut(pc, "/")
			if out[p] == nil {
				out[p] = map[string]bool{}
			}
			out[p][c] = true
		}
		return out
	}
	machines := []*Machine{
		{Name: "alice-mbp", Member: "alice", Fingerprint: "age1q8…7xk", Addr: "100.64.0.11", This: true, Online: true,
			Grants: g("vet45-erp/dev", "vet45-erp/dev_alice", "vet45-erp/stg", "vet45-erp/prd", "vet45-shared/dev", "mau-hien-app/dev")},
		{Name: "alice-mini", Member: "alice", Fingerprint: "age1z3…p0d", Addr: "100.64.0.12", Online: true, Behind: 1, LastSeen: "now",
			Grants: g("vet45-erp/dev", "vet45-erp/dev_alice")},
		{Name: "bob-mbp", Member: "bob", Fingerprint: "age1m2…c4r", Addr: "100.64.0.21", Online: true, LastSeen: "now",
			Grants: g("vet45-erp/dev", "vet45-erp/stg", "vet45-shared/dev", "vet45-shared/prd", "mau-hien-app/dev", "mau-hien-app/prd")},
		{Name: "carol-linux", Member: "carol", Fingerprint: "age1k9…e1a", Addr: "100.64.0.31", Online: false, Behind: 3, LastSeen: "2d ago",
			Grants: g("vet45-erp/dev")},
		{Name: "bob-old-laptop", Member: "bob", Fingerprint: "age1h4…w8n", Addr: "100.64.0.22", Revoked: true, LastSeen: "9d ago",
			Grants: g()},
		{Name: "dan-thinkpad", Member: "dan", Fingerprint: "age1t7…y2m", Addr: "100.64.0.41", JoinRequest: true, LastSeen: "1m ago",
			Grants: g()},
	}
	return &Store{
		Projects: []*Project{erp, shared, mau},
		Machines: machines,
		Active:   map[string]string{"vet45-erp": "dev", "vet45-shared": "dev", "mau-hien-app": "dev"},
		LastSync: "12s ago",
		Local:    0,
		Log: []string{
			"Sync    pulled 4 Changes from bob-mbp",
			"Conflict STRIPE_KEY on vet45-erp/dev: bob-mbp vs alice-mini (run uses bob-mbp's, newer HLC)",
			"Dropped 2 Changes from revoked bob-old-laptop on vet45-erp/stg",
			"Project revoke bob-old-laptop (signed by alice) · vet45-erp/prd key rotation pending",
			"Join    dan-thinkpad requests to join vet45-erp · fingerprint age1t7…y2m",
		},
	}
}

// Missing returns the Setup `requires` keys absent from the given Config.
func (p *Project) Missing(c *Config) []string {
	var out []string
	for _, k := range p.Requires {
		if c.Find(k) == nil {
			out = append(out, k)
		}
	}
	return out
}

func (cmd Command) Allowed(c *Config) bool {
	if len(cmd.Envs) == 0 {
		return true
	}
	for _, e := range cmd.Envs {
		if e == c.Env {
			return true
		}
	}
	return false
}
