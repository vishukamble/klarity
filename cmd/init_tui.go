package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vishukamble/klarity/pkg/config"
)

// ── Colour palette (website dark theme) ─────────────────────────────────────
// These vars are shared with config_tui.go (same package).

var (
	tuiBlue   = lipgloss.Color("#58A6FF")
	tuiRed    = lipgloss.Color("#F85149")
	tuiGreen  = lipgloss.Color("#3FB950")
	tuiAmber  = lipgloss.Color("#F59E0B")
	tuiDim    = lipgloss.Color("#484F58")
	tuiText   = lipgloss.Color("#E6EDF3")
	tuiBorder = lipgloss.Color("#30363D")
	tuiSelBg  = lipgloss.Color("#161B22")
)

var (
	tuiLogo       = lipgloss.NewStyle().Foreground(tuiBlue).Bold(true)
	tuiStyleBlue  = lipgloss.NewStyle().Foreground(tuiBlue)
	tuiStyleRed   = lipgloss.NewStyle().Foreground(tuiRed)
	tuiStyleGreen = lipgloss.NewStyle().Foreground(tuiGreen)
	tuiStyleDim   = lipgloss.NewStyle().Foreground(tuiDim)
	tuiStyleText  = lipgloss.NewStyle().Foreground(tuiText)
	tuiStyleSel   = lipgloss.NewStyle().Background(tuiSelBg).Foreground(tuiText)
	tuiStyleBold  = lipgloss.NewStyle().Bold(true).Foreground(tuiText)
	tuiStyleBox   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tuiBorder).
			Padding(0, 1)
	tuiStyleAmber = lipgloss.NewStyle().Foreground(tuiAmber)
	tuiStyleHelp  = lipgloss.NewStyle().Foreground(tuiDim)
)

// Phase display metadata.
var tuiPhaseLabels = []string{"Groupings", "Assign", "Tiers", "Default", "Save"}

var tuiPhaseTitles = []string{
	"Step 1 of 5 — Review auto-detected groupings",
	"Step 2 of 5 — Assign unmatched clusters",
	"Step 3 of 5 — Confirm tier assignments",
	"Step 4 of 5 — Set default environment",
	"Step 5 of 5 — Save configuration",
}

var tuiHelpBars = []string{
	"  ↑↓ move  ·  enter accept  ·  e edit  ·  esc cancel",
	"  ↑↓ move  ·  enter select  ·  type to rename  ·  esc back",
	"  ↑↓ move  ·  space toggle  ·  enter confirm  ·  esc back",
	"  ↑↓ move  ·  enter select  ·  esc back",
	"  enter save  ·  esc back  ·  q quit",
}

// ── Model ────────────────────────────────────────────────────────────────────

type initTUIModel struct {
	phase           int // 0=Groupings 1=Assign 2=Tiers 3=Default 4=Save
	detected        config.DetectedEnvs
	envOrder        []string            // preserves env ordering
	selected        map[string][]string // env → clusters
	unmatched       []string            // unmatched clusters from detection
	unmatchedIdx    int                 // which unmatched cluster is being assigned
	unmatchedAssign map[string]string   // cluster → env name
	tiers           map[string]string   // env → "critical"|"standard"
	tierCursor      int
	defaultEnv      string
	defCursor       int
	cursor          int    // generic list cursor
	inputVal        string // live text input for "new group"
	inputActive     bool   // true while typing a group name
	firstRun        bool   // show tip banner
	tipDismissed    bool
	defaults        *config.Config
	resultCfg       *config.Config // set on save; nil means cancelled
	done            bool
	quitting        bool
	editMsg         string // transient message for unsupported actions
	width, height   int
}

func newInitTUIModel(detected config.DetectedEnvs, defaults *config.Config, firstRun bool) initTUIModel {
	selected := make(map[string][]string, len(detected.Order))
	order := make([]string, len(detected.Order))
	copy(order, detected.Order)
	for _, label := range detected.Order {
		selected[label] = append([]string(nil), detected.Envs[label]...)
	}
	tiers := make(map[string]string, len(detected.Order))
	for _, label := range detected.Order {
		tiers[label] = config.InferTier(label)
	}
	return initTUIModel{
		phase:           0,
		detected:        detected,
		envOrder:        order,
		selected:        selected,
		unmatched:       detected.Unmatched,
		unmatchedAssign: make(map[string]string),
		tiers:           tiers,
		firstRun:        firstRun,
		defaults:        defaults,
	}
}

func (m initTUIModel) Init() tea.Cmd { return nil }

// nextPhase advances, skipping phase 1 (unmatched) when there are none.
func (m initTUIModel) nextPhase() int {
	if m.phase == 0 && len(m.unmatched) == 0 {
		return 2
	}
	return m.phase + 1
}

// prevPhase goes back, skipping phase 1 (unmatched) when there are none.
func (m initTUIModel) prevPhase() int {
	if m.phase == 2 && len(m.unmatched) == 0 {
		return 0
	}
	return m.phase - 1
}

// activeEnvOrder returns envs that have at least one cluster assigned.
func (m initTUIModel) activeEnvOrder() []string {
	var out []string
	for _, label := range m.envOrder {
		if len(m.selected[label]) > 0 {
			out = append(out, label)
		}
	}
	return out
}

// phase1Options returns the cursor options for unmatched assignment:
// existing groups + "→ new group" + "skip (exclude from config)".
func (m initTUIModel) phase1Options() []string {
	opts := make([]string, 0, len(m.envOrder)+2)
	opts = append(opts, m.envOrder...)
	opts = append(opts, "→ new group")
	opts = append(opts, "skip (exclude from config)")
	return opts
}

func (m initTUIModel) buildConfig() *config.Config {
	cfg := config.BuildDetectedConfigWithTiers(m.selected, m.envOrder, m.tiers, m.defaults)
	cfg.Settings.DefaultEnv = m.defaultEnv
	return cfg
}

// ── Update ───────────────────────────────────────────────────────────────────

func (m initTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Dismiss tip on any key while viewing phase 0.
		if m.phase == 0 && m.firstRun && !m.tipDismissed {
			m.tipDismissed = true
		}

		// ctrl+c always quits without saving.
		if msg.Type == tea.KeyCtrlC {
			m.done = true
			m.quitting = true
			return m, tea.Quit
		}

		// esc: dismiss active input first, then navigate back.
		if msg.Type == tea.KeyEsc {
			if m.inputActive {
				m.inputActive = false
				m.inputVal = ""
				return m, nil
			}
			if m.phase == 0 {
				m.done = true
				m.quitting = true
				return m, tea.Quit
			}
			m.editMsg = ""
			m.phase = m.prevPhase()
			m.cursor = 0
			return m, nil
		}

		// q quits when not entering text.
		if !m.inputActive && msg.Type == tea.KeyRunes && string(msg.Runes) == "q" {
			m.done = true
			m.quitting = true
			return m, tea.Quit
		}

		switch m.phase {
		case 0:
			return m.updateGroupings(msg)
		case 1:
			return m.updateUnmatched(msg)
		case 2:
			return m.updateTiers(msg)
		case 3:
			return m.updateDefaultEnv(msg)
		case 4:
			return m.updateSave(msg)
		}
	}
	return m, nil
}

func (m initTUIModel) updateGroupings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.envOrder)
	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if n > 0 && m.cursor < n-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		m.editMsg = ""
		m.phase = m.nextPhase()
		m.cursor = 0
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "y", "Y":
			m.editMsg = ""
			m.phase = m.nextPhase()
			m.cursor = 0
		case "e", "E":
			m.editMsg = "Edit mode not yet available — use 'klarity init' manual assignment"
		}
	}
	return m, nil
}

func (m initTUIModel) updateUnmatched(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.unmatchedIdx >= len(m.unmatched) {
		return m, nil
	}
	opts := m.phase1Options()
	newGroupIdx := len(m.envOrder)
	skipIdx := len(m.envOrder) + 1

	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.inputActive = m.cursor == newGroupIdx
		}
	case tea.KeyDown:
		if m.cursor < len(opts)-1 {
			m.cursor++
			m.inputActive = m.cursor == newGroupIdx
		}
	case tea.KeyEnter:
		cluster := m.unmatched[m.unmatchedIdx]
		switch {
		case m.cursor == newGroupIdx:
			groupName := strings.TrimSpace(m.inputVal)
			if groupName == "" {
				return m, nil // need a name first
			}
			if _, exists := m.selected[groupName]; !exists {
				m.envOrder = append(m.envOrder, groupName)
				m.tiers[groupName] = config.InferTier(groupName)
				m.selected[groupName] = nil
			}
			m.selected[groupName] = append(m.selected[groupName], cluster)
			m.unmatchedAssign[cluster] = groupName
		case m.cursor == skipIdx:
			// skip — not assigned to any env
		case m.cursor < len(m.envOrder):
			envName := m.envOrder[m.cursor]
			m.selected[envName] = append(m.selected[envName], cluster)
			m.unmatchedAssign[cluster] = envName
		}
		m.unmatchedIdx++
		m.cursor = 0
		m.inputVal = ""
		m.inputActive = false
		if m.unmatchedIdx >= len(m.unmatched) {
			m.phase = 2
			m.tierCursor = 0
		}
	case tea.KeyBackspace:
		if m.inputActive && len(m.inputVal) > 0 {
			r := []rune(m.inputVal)
			m.inputVal = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		if m.inputActive {
			m.inputVal += string(msg.Runes)
		}
	}
	return m, nil
}

func (m initTUIModel) updateTiers(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	active := m.activeEnvOrder()
	switch msg.Type {
	case tea.KeyUp:
		if m.tierCursor > 0 {
			m.tierCursor--
		}
	case tea.KeyDown:
		if len(active) > 0 && m.tierCursor < len(active)-1 {
			m.tierCursor++
		}
	case tea.KeySpace:
		if m.tierCursor < len(active) {
			name := active[m.tierCursor]
			if m.tiers[name] == config.TierCritical {
				m.tiers[name] = config.TierStandard
			} else {
				m.tiers[name] = config.TierCritical
			}
		}
	case tea.KeyEnter:
		m.phase = 3
		m.defCursor = 0
	}
	return m, nil
}

func (m initTUIModel) updateDefaultEnv(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	active := m.activeEnvOrder()
	total := len(active) + 1 // +1 for "No default" option
	switch msg.Type {
	case tea.KeyUp:
		if m.defCursor > 0 {
			m.defCursor--
		}
	case tea.KeyDown:
		if m.defCursor < total-1 {
			m.defCursor++
		}
	case tea.KeyEnter:
		if m.defCursor < len(active) {
			m.defaultEnv = active[m.defCursor]
		} else {
			m.defaultEnv = ""
		}
		m.phase = 4
	}
	return m, nil
}

func (m initTUIModel) updateSave(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.resultCfg = m.buildConfig()
		m.done = true
		return m, tea.Quit
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "s", "S", "n", "N":
			m.resultCfg = m.buildConfig()
			m.done = true
			return m, tea.Quit
		case "b", "B":
			m.phase = 3
		}
	}
	return m, nil
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m initTUIModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")
	if m.phase == 0 && m.firstRun && !m.tipDismissed {
		sb.WriteString(m.renderTipBanner())
		sb.WriteString("\n\n")
	}
	sb.WriteString(m.renderPhaseTitle())
	sb.WriteString("\n\n")
	switch m.phase {
	case 0:
		sb.WriteString(m.renderGroupings())
	case 1:
		sb.WriteString(m.renderUnmatched())
	case 2:
		sb.WriteString(m.renderTiersPhase())
	case 3:
		sb.WriteString(m.renderDefaultEnvPhase())
	case 4:
		sb.WriteString(m.renderSave())
	}
	if m.editMsg != "" {
		sb.WriteString("\n\n")
		sb.WriteString(tuiStyleAmber.Render("  " + m.editMsg))
	}
	sb.WriteString("\n\n")
	sb.WriteString(m.renderHelpBar())
	return sb.String()
}

func (m initTUIModel) renderHeader() string {
	logo := tuiLogo.Render("  klarity")
	pills := m.renderPhasePills()
	if m.width > 0 {
		lw := lipgloss.Width(logo)
		pw := lipgloss.Width(pills)
		if gap := m.width - lw - pw; gap > 0 {
			return logo + strings.Repeat(" ", gap) + pills
		}
	}
	return logo + strings.Repeat(" ", 6) + pills
}

func (m initTUIModel) renderPhasePills() string {
	parts := make([]string, len(tuiPhaseLabels))
	for i, label := range tuiPhaseLabels {
		switch {
		case i < m.phase:
			parts[i] = tuiStyleGreen.Render("●")
		case i == m.phase:
			parts[i] = tuiStyleBlue.Render("● " + label)
		default:
			parts[i] = tuiStyleDim.Render("○")
		}
	}
	return strings.Join(parts, "  ")
}

func (m initTUIModel) renderPhaseTitle() string {
	return tuiStyleBold.Render("  " + tuiPhaseTitles[m.phase])
}

func (m initTUIModel) renderTipBanner() string {
	content := "─ tip\n" +
		"  klarity init is keyboard-driven. ↑↓ move, enter confirm,\n" +
		"  esc goes back. Controls are shown at the bottom."
	return tuiStyleBox.Render(content)
}

func (m initTUIModel) renderGroupings() string {
	var sb strings.Builder
	if len(m.envOrder) == 0 {
		sb.WriteString(tuiStyleDim.Render("  No environments detected."))
		return sb.String()
	}

	sb.WriteString(tuiStyleDim.Render(fmt.Sprintf("  %-24s %-10s  %s", "Environment", "Tier", "Clusters")))
	sb.WriteString("\n")
	sb.WriteString(tuiStyleDim.Render("  " + strings.Repeat("─", 55)))
	sb.WriteString("\n")

	for i, label := range m.envOrder {
		clusters := m.detected.Envs[label]
		tier := config.InferTier(label)
		var clusterStr string
		if len(clusters) <= 3 {
			clusterStr = strings.Join(clusters, ", ")
		} else {
			clusterStr = fmt.Sprintf("%d clusters", len(clusters))
		}
		rowBase := fmt.Sprintf("  %-24s %-10s  %s", label, tier, clusterStr)
		switch {
		case i == m.cursor:
			sb.WriteString(tuiStyleSel.Render(rowBase))
		case tier == config.TierCritical:
			sb.WriteString(tuiStyleRed.Render(rowBase))
		default:
			sb.WriteString(tuiStyleText.Render(rowBase))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	if len(m.unmatched) > 0 {
		word := "cluster"
		if len(m.unmatched) != 1 {
			word = "clusters"
		}
		sb.WriteString(tuiStyleAmber.Render(
			fmt.Sprintf("  %d %s unmatched — assign in next step", len(m.unmatched), word),
		))
	} else {
		sb.WriteString(tuiStyleGreen.Render("  ✓ All clusters matched"))
	}
	return sb.String()
}

func (m initTUIModel) renderUnmatched() string {
	if m.unmatchedIdx >= len(m.unmatched) {
		return tuiStyleGreen.Render("  ✓ All unmatched clusters assigned")
	}
	var sb strings.Builder
	cluster := m.unmatched[m.unmatchedIdx]
	total := len(m.unmatched)
	opts := m.phase1Options()
	newGroupIdx := len(m.envOrder)

	sb.WriteString(tuiStyleDim.Render(fmt.Sprintf("  Cluster %d of %d", m.unmatchedIdx+1, total)))
	sb.WriteString("\n\n")
	sb.WriteString(tuiStyleBold.Render("  " + cluster))
	sb.WriteString("\n\n")

	for i, opt := range opts {
		cur := "  "
		if i == m.cursor {
			cur = tuiStyleBlue.Render("› ")
		}
		if i == newGroupIdx {
			inputDisplay := m.inputVal
			if inputDisplay == "" {
				inputDisplay = tuiStyleDim.Render("type group name...")
			}
			sb.WriteString("  " + cur + tuiStyleText.Render("→ new group: ") + inputDisplay)
		} else if i == m.cursor {
			sb.WriteString("  " + cur + tuiStyleSel.Render(opt))
		} else {
			sb.WriteString("  " + cur + tuiStyleText.Render(opt))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m initTUIModel) renderTiersPhase() string {
	var sb strings.Builder
	active := m.activeEnvOrder()
	sb.WriteString(tuiStyleDim.Render(fmt.Sprintf("  %-24s %-12s", "Environment", "Tier")))
	sb.WriteString("\n")
	sb.WriteString(tuiStyleDim.Render("  " + strings.Repeat("─", 38)))
	sb.WriteString("\n")
	for i, label := range active {
		tier := m.tiers[label]
		base := fmt.Sprintf("  %-24s %-12s", label, tier)
		var row string
		switch {
		case i == m.tierCursor:
			row = tuiStyleSel.Render(base) + tuiStyleDim.Render("  ← space to toggle")
		case tier == config.TierCritical:
			row = tuiStyleRed.Render(base)
		default:
			row = tuiStyleText.Render(base)
		}
		sb.WriteString(row + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(tuiStyleDim.Render("  ● critical = shown first, red header  ○ standard = default"))
	return sb.String()
}

func (m initTUIModel) renderDefaultEnvPhase() string {
	var sb strings.Builder
	active := m.activeEnvOrder()
	sb.WriteString(tuiStyleText.Render("  klarity will scan this by default. Use --env for others."))
	sb.WriteString("\n\n")
	for i, label := range active {
		tier := m.tiers[label]
		tierTag := "[standard]"
		if tier == config.TierCritical {
			tierTag = "[critical]"
		}
		row := fmt.Sprintf("%-24s %s", label, tierTag)
		if i == m.defCursor {
			sb.WriteString(tuiStyleBlue.Render("  › ") + tuiStyleSel.Render(row))
		} else if tier == config.TierCritical {
			sb.WriteString("    " + tuiStyleRed.Render(row))
		} else {
			sb.WriteString("    " + tuiStyleText.Render(row))
		}
		sb.WriteString("\n")
	}
	noDefaultRow := "No default — scan everything"
	if m.defCursor == len(active) {
		sb.WriteString(tuiStyleBlue.Render("  › ") + tuiStyleSel.Render(noDefaultRow))
	} else {
		sb.WriteString("    " + tuiStyleDim.Render(noDefaultRow))
	}
	sb.WriteString("\n")
	return sb.String()
}

func (m initTUIModel) renderSave() string {
	var sb strings.Builder
	active := m.activeEnvOrder()
	clusterCount := 0
	for _, label := range active {
		clusterCount += len(m.selected[label])
	}
	cfgPath, _ := config.ConfigPath()
	defDisplay := m.defaultEnv
	if defDisplay == "" {
		defDisplay = "none"
	}
	nsExclude := strings.Join(m.defaults.Settings.DefaultNsExclude, ", ")
	summary := fmt.Sprintf(
		"  environments:  %d\n  clusters:      %d\n  default env:   %s\n  namespaces:    all (excluding %s)\n  config path:   %s",
		len(active), clusterCount, defDisplay, nsExclude, cfgPath,
	)
	sb.WriteString(tuiStyleBox.Render(summary))
	sb.WriteString("\n\n")
	sb.WriteString(
		tuiStyleText.Render("  ") +
			tuiStyleGreen.Render("enter") +
			tuiStyleText.Render(" to save  ·  ") +
			tuiStyleDim.Render("esc") +
			tuiStyleText.Render(" to go back"),
	)
	return sb.String()
}

func (m initTUIModel) renderHelpBar() string {
	rule := tuiStyleHelp.Render(strings.Repeat("─", 60))
	return rule + "\n" + tuiStyleHelp.Render(tuiHelpBars[m.phase])
}

// ── Entry point ───────────────────────────────────────────────────────────────

// runNewWizardTUI launches the full-screen Bubbletea init wizard and returns
// the resulting config (nil if the user aborted without saving).
func runNewWizardTUI(detected config.DetectedEnvs, defaults *config.Config, firstRun bool) (*config.Config, error) {
	if len(detected.Order) == 0 && len(detected.Unmatched) == 0 {
		return nil, fmt.Errorf("no clusters to configure")
	}
	m := newInitTUIModel(detected, defaults, firstRun)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalRaw, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("init TUI: %w", err)
	}
	result := finalRaw.(initTUIModel)
	if result.quitting || result.resultCfg == nil {
		return nil, nil
	}
	return result.resultCfg, nil
}
