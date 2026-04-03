package cmd

// config_tui.go — Bubbletea interactive menu for `klarity config` (no subcommand).
// Colour/style vars are defined in init_tui.go (same package).

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vishukamble/klarity/pkg/config"
)

// ── Menu action constants ─────────────────────────────────────────────────────

type configMenuAction int

const (
	cfgActionNone     configMenuAction = iota
	cfgActionShow                      // exit TUI → print config summary
	cfgActionValidate                  // exit TUI → run validate logic
	cfgActionReinit                    // exit TUI → re-run init wizard
	cfgActionExit                      // quit
)

// editorFinishedMsg is sent by ExecProcess after the external editor exits.
type editorFinishedMsg struct{ err error }

// Cursor indices into configMenuItems.
const (
	cfgItemEdit          = 1
	cfgItemChangeDefault = 3
)

var configMenuItems = []struct {
	label  string
	action configMenuAction
}{
	{"Show config", cfgActionShow},
	{"Edit in $EDITOR", cfgActionNone},
	{"Validate cluster connections", cfgActionValidate},
	{"Change default environment", cfgActionNone},
	{"Re-run init wizard", cfgActionReinit},
	{"Exit", cfgActionExit},
}

// ── Model ────────────────────────────────────────────────────────────────────

type configMenuModel struct {
	cursor          int
	action          configMenuAction
	done            bool
	cfg             *config.Config
	cfgPath         string
	loadErr         error
	changingDefault bool   // true while the inline env-list is shown
	defCursor       int    // cursor within the default-env sub-list
	defEnvs         []string
	width, height   int
	infoMsg         string
	errMsg          string
}

func newConfigMenuModel(cfg *config.Config, cfgPath string, loadErr error) configMenuModel {
	var defEnvs []string
	if cfg != nil {
		for _, env := range cfg.Environments {
			defEnvs = append(defEnvs, env.Name)
		}
	}
	return configMenuModel{
		cfg:     cfg,
		cfgPath: cfgPath,
		loadErr: loadErr,
		defEnvs: defEnvs,
	}
}

func (m configMenuModel) Init() tea.Cmd { return nil }

// ── Update ───────────────────────────────────────────────────────────────────

func (m configMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Editor error: %v", msg.err)
		} else {
			newCfg, err := config.Load(m.cfgPath)
			if err != nil {
				m.errMsg = fmt.Sprintf("Config error after editing: %v", err)
			} else {
				m.cfg = newCfg
				m.defEnvs = nil
				for _, env := range newCfg.Environments {
					m.defEnvs = append(m.defEnvs, env.Name)
				}
				m.infoMsg = "Config updated successfully."
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.changingDefault {
			return m.updateChangeDefault(msg)
		}

		// Global quit keys.
		switch {
		case msg.Type == tea.KeyCtrlC:
			m.done = true
			m.action = cfgActionExit
			return m, tea.Quit
		case msg.Type == tea.KeyEsc:
			m.done = true
			m.action = cfgActionExit
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			m.done = true
			m.action = cfgActionExit
			return m, tea.Quit
		}

		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			m.infoMsg, m.errMsg = "", ""
		case tea.KeyDown:
			if m.cursor < len(configMenuItems)-1 {
				m.cursor++
			}
			m.infoMsg, m.errMsg = "", ""
		case tea.KeyEnter:
			return m.handleMenuSelect()
		}
	}
	return m, nil
}

func (m configMenuModel) handleMenuSelect() (tea.Model, tea.Cmd) {
	m.infoMsg, m.errMsg = "", ""

	switch m.cursor {
	case cfgItemEdit:
		if m.cfg == nil {
			m.errMsg = "No config found. Run 'klarity init' first."
			return m, nil
		}
		editor := findEditor()
		if editor == "" {
			m.errMsg = "No editor found. Set $EDITOR to your preferred editor."
			return m, nil
		}
		c := exec.Command(editor, m.cfgPath) //nolint:gosec
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return editorFinishedMsg{err: err}
		})

	case cfgItemChangeDefault:
		if m.cfg == nil {
			m.errMsg = "No config found. Run 'klarity init' first."
			return m, nil
		}
		m.changingDefault = true
		// Pre-select the currently configured default env, or "no default".
		m.defCursor = len(m.defEnvs)
		for i, name := range m.defEnvs {
			if name == m.cfg.Settings.DefaultEnv {
				m.defCursor = i
				break
			}
		}
		return m, nil

	default:
		item := configMenuItems[m.cursor]
		if item.action != cfgActionNone {
			m.done = true
			m.action = item.action
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m configMenuModel) updateChangeDefault(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := len(m.defEnvs) + 1 // +1 for "No default"
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
		if m.defCursor < len(m.defEnvs) {
			m.cfg.Settings.DefaultEnv = m.defEnvs[m.defCursor]
		} else {
			m.cfg.Settings.DefaultEnv = ""
		}
		if err := config.Save(m.cfg, m.cfgPath); err != nil {
			m.errMsg = fmt.Sprintf("Save error: %v", err)
		} else {
			defDisplay := m.cfg.Settings.DefaultEnv
			if defDisplay == "" {
				defDisplay = "none"
			}
			m.infoMsg = fmt.Sprintf("Default env set to %q.", defDisplay)
		}
		m.changingDefault = false
	case tea.KeyEsc:
		m.changingDefault = false
	}
	return m, nil
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m configMenuModel) View() string {
	boxWidth := 100
	if m.width > 0 {
		boxWidth = min(m.width-4, 100)
	}
	boxStyle := lipgloss.NewStyle().Width(boxWidth)

	var sb strings.Builder

	sb.WriteString(tuiLogo.Render("  klarity"))
	sb.WriteString("\n\n")
	sb.WriteString(m.renderConfigSummary())
	sb.WriteString("\n\n")

	if m.changingDefault {
		sb.WriteString(m.renderChangeDefault())
	} else {
		sb.WriteString(m.renderMenu())
	}

	if m.infoMsg != "" {
		sb.WriteString("\n\n")
		sb.WriteString(tuiStyleGreen.Render("  ✓ " + m.infoMsg))
	}
	if m.errMsg != "" {
		sb.WriteString("\n\n")
		sb.WriteString(tuiStyleRed.Render("  ✗ " + m.errMsg))
	}

	sb.WriteString("\n\n")
	sb.WriteString(tuiStyleHelp.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n")
	if m.changingDefault {
		sb.WriteString(tuiStyleHelp.Render("  ↑↓ move  ·  enter select  ·  esc cancel"))
	} else {
		sb.WriteString(tuiStyleHelp.Render("  ↑↓ move  ·  enter select  ·  q quit"))
	}

	content := boxStyle.Render(sb.String())
	if m.width <= 0 {
		return content
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, content)
}

func (m configMenuModel) renderConfigSummary() string {
	if m.loadErr != nil || m.cfg == nil {
		return tuiStyleAmber.Render("  No config found. Run 'klarity init' to get started.")
	}
	envCount := len(m.cfg.Environments)
	clusterCount := 0
	for _, env := range m.cfg.Environments {
		clusterCount += len(env.Clusters)
	}
	defDisplay := m.cfg.Settings.DefaultEnv
	if defDisplay == "" {
		defDisplay = "none"
	}
	return tuiStyleDim.Render(fmt.Sprintf(
		"  Config: %s\n  Environments: %d  ·  Clusters: %d  ·  Default: %s",
		m.cfgPath, envCount, clusterCount, defDisplay,
	))
}

func (m configMenuModel) renderMenu() string {
	var lines []string
	for i, item := range configMenuItems {
		var line string
		if i == m.cursor {
			line = tuiStyleBlue.Render("›") + " " + tuiStyleSel.Render(item.label)
		} else {
			line = "  " + tuiStyleText.Render(item.label)
		}
		lines = append(lines, line)
	}
	return tuiStyleBox.Render(strings.Join(lines, "\n"))
}

func (m configMenuModel) renderChangeDefault() string {
	var sb strings.Builder
	sb.WriteString(tuiStyleBold.Render("  Select default environment:"))
	sb.WriteString("\n\n")
	for i, name := range m.defEnvs {
		if i == m.defCursor {
			sb.WriteString(tuiStyleBlue.Render("  › ") + tuiStyleSel.Render(name))
		} else {
			sb.WriteString("    " + tuiStyleText.Render(name))
		}
		sb.WriteString("\n")
	}
	noDefLabel := "No default — scan everything"
	if m.defCursor == len(m.defEnvs) {
		sb.WriteString(tuiStyleBlue.Render("  › ") + tuiStyleSel.Render(noDefLabel))
	} else {
		sb.WriteString("    " + tuiStyleDim.Render(noDefLabel))
	}
	sb.WriteString("\n")
	return sb.String()
}
