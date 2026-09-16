// Package wizard implements the orchard init interactive setup wizard.
// It walks the user through first-time configuration: running orchard setup,
// validating with orchard doctor, and optionally creating config.toml.
package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/doctor"
	"github.com/hcarminati/orchard/internal/setup"
)

// Step identifies the current wizard step.
type Step int

const (
	stepWelcome   Step = iota // welcome screen
	stepSetup                 // run orchard setup
	stepDoctor                // run orchard doctor
	stepConfig                // offer to create config.toml
	stepDone                  // all done
)

// configField tracks a user-editable config value.
type configField struct {
	label   string
	hint    string
	value   string
}

// Model is the Bubbletea model for the setup wizard.
type Model struct {
	step         Step
	width        int
	height       int
	port         int
	settingsPath string

	// step 1: setup results
	setupDone bool
	setupErr  string

	// step 2: doctor results
	checks []doctor.Check

	// step 3: config fields
	fields       []configField
	activeField  int
	configSaved  bool
	configPath   string

	// general
	quitting bool
	logs     []string // status messages accumulated across steps
}

// Result is returned by Run() to indicate overall outcome.
type Result struct {
	Completed bool   // true if the user reached the Done step
	ConfigPath string // non-empty if config.toml was written
}

var (
	colorAccent = lipgloss.Color("#7C3AED")
	colorMuted  = lipgloss.Color("#6B7280")
	colorFg     = lipgloss.Color("#F9FAFB")
	colorGreen  = lipgloss.Color("#10B981")
	colorRed    = lipgloss.Color("#EF4444")
	colorYellow = lipgloss.Color("#F59E0B")
	colorBg     = lipgloss.Color("#111827")

	styleTitle   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(colorMuted)
	styleFg      = lipgloss.NewStyle().Foreground(colorFg)
	styleGreen   = lipgloss.NewStyle().Foreground(colorGreen)
	styleRed     = lipgloss.NewStyle().Foreground(colorRed)
	styleYellow  = lipgloss.NewStyle().Foreground(colorYellow)
	styleFooter  = lipgloss.NewStyle().Background(colorBg).Foreground(colorMuted)
	styleInput   = lipgloss.NewStyle().Foreground(colorFg).Underline(true)
	styleLabel   = lipgloss.NewStyle().Foreground(colorAccent)
)

// setupMsg carries the result of running orchard setup.
type setupMsg struct {
	err string
}

// doctorMsg carries health check results.
type doctorMsg struct {
	checks []doctor.Check
}

// NewModel creates a wizard Model.
func NewModel(port int, settingsPath string, configPath string) Model {
	return Model{
		port:         port,
		settingsPath: settingsPath,
		configPath:   configPath,
		width:        80,
		height:       24,
		fields: []configField{
			{label: "Budget (USD/month)", hint: "e.g. 50 — shows cost bar; 0 to disable", value: ""},
			{label: "Watchdog (minutes)", hint: "e.g. 5 — alerts when agent is stuck; 0 to disable", value: ""},
			{label: "Cost alert (USD)", hint: "e.g. 1.0 — shows banner when session exceeds this; 0 to disable", value: ""},
		},
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

// runSetupCmd performs orchard setup in the background.
func (m Model) runSetupCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := setup.Run(m.settingsPath, setup.Options{
			Port: m.port,
		})
		if err != nil {
			return setupMsg{err: err.Error()}
		}
		return setupMsg{}
	}
}

// runDoctorCmd runs health checks.
func (m Model) runDoctorCmd() tea.Cmd {
	return func() tea.Msg {
		return doctorMsg{checks: doctor.RunAll(m.port)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case setupMsg:
		m.setupDone = true
		if msg.err != "" {
			m.setupErr = msg.err
			m.logs = append(m.logs, styleRed.Render("✗ setup failed: "+msg.err))
		} else {
			m.logs = append(m.logs, styleGreen.Render("✓ hooks merged into settings.json"))
		}
		// Automatically advance to doctor step.
		m.step = stepDoctor
		return m, m.runDoctorCmd()

	case doctorMsg:
		m.checks = msg.checks
		m.step = stepConfig
		for _, c := range msg.checks {
			m.logs = append(m.logs, c.String())
		}
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case stepWelcome:
			switch msg.String() {
			case "enter", " ":
				m.step = stepSetup
				return m, m.runSetupCmd()
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}

		case stepSetup:
			// Waiting for setup to complete — no key handling except quit.
			if msg.String() == "ctrl+c" {
				m.quitting = true
				return m, tea.Quit
			}

		case stepDoctor:
			// Waiting for doctor to complete.
			if msg.String() == "ctrl+c" {
				m.quitting = true
				return m, tea.Quit
			}

		case stepConfig:
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case "tab", "down":
				m.activeField = (m.activeField + 1) % len(m.fields)
			case "shift+tab", "up":
				m.activeField = (m.activeField - 1 + len(m.fields)) % len(m.fields)
			case "enter":
				// Save config and advance.
				if err := m.writeConfig(); err != nil {
					m.logs = append(m.logs, styleRed.Render("✗ config write failed: "+err.Error()))
				} else {
					m.configSaved = true
					m.logs = append(m.logs, styleGreen.Render("✓ config saved to "+m.configPath))
				}
				m.step = stepDone
				return m, nil
			case "s":
				// Skip config step.
				m.step = stepDone
				return m, nil
			case "backspace", "ctrl+h":
				f := &m.fields[m.activeField]
				if len(f.value) > 0 {
					runes := []rune(f.value)
					f.value = string(runes[:len(runes)-1])
				}
			default:
				s := msg.String()
				if len(s) == 1 && (s[0] >= '0' && s[0] <= '9' || s[0] == '.') {
					m.fields[m.activeField].value += s
				}
			}

		case stepDone:
			switch msg.String() {
			case "q", "ctrl+c", "enter", " ":
				m.quitting = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder

	// Header
	sb.WriteString(styleTitle.Render("  orchard init"))
	sb.WriteString(styleMuted.Render(" — first-time setup"))
	sb.WriteString("\n\n")

	switch m.step {
	case stepWelcome:
		sb.WriteString(styleFg.Render("  Welcome to Orchard! This wizard will:"))
		sb.WriteString("\n\n")
		sb.WriteString(styleMuted.Render("  1. Merge Orchard hooks into ~/.claude/settings.json"))
		sb.WriteString("\n")
		sb.WriteString(styleMuted.Render("  2. Run health checks to verify the setup"))
		sb.WriteString("\n")
		sb.WriteString(styleMuted.Render("  3. (Optional) Create ~/.config/orchard/config.toml"))
		sb.WriteString("\n\n")
		sb.WriteString(styleMuted.Render("  Press enter to begin, or q to skip."))

	case stepSetup:
		sb.WriteString(styleFg.Render("  Running orchard setup…"))
		sb.WriteString("\n")
		if m.setupDone {
			if m.setupErr != "" {
				sb.WriteString(styleRed.Render("  ✗ " + m.setupErr))
			} else {
				sb.WriteString(styleGreen.Render("  ✓ Done"))
			}
		}

	case stepDoctor:
		sb.WriteString(styleFg.Render("  Running health checks…"))
		sb.WriteString("\n")
		for _, c := range m.checks {
			sb.WriteString("  " + c.String() + "\n")
		}

	case stepConfig:
		sb.WriteString(styleFg.Render("  Configure orchard (optional):"))
		sb.WriteString("\n\n")
		for i, f := range m.fields {
			active := i == m.activeField
			label := styleMuted.Render(f.label+": ")
			if active {
				label = styleLabel.Render(f.label+": ")
			}
			val := f.value
			if val == "" {
				val = styleMuted.Render("(press tab to focus, type number)")
			} else if active {
				val = styleInput.Render(val) + styleYellow.Render("█")
			}
			sb.WriteString("  " + label + val + "\n")
			if active {
				sb.WriteString(styleMuted.Render("    "+f.hint) + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(styleMuted.Render("  enter to save · s to skip · tab to move between fields"))

	case stepDone:
		sb.WriteString(styleGreen.Render("  ✓ Setup complete!"))
		sb.WriteString("\n\n")
		if m.configSaved {
			sb.WriteString(styleFg.Render("  Config saved: "+m.configPath))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		sb.WriteString(styleFg.Render("  Start Orchard:"))
		sb.WriteString("\n")
		sb.WriteString(styleMuted.Render("    orchard"))
		sb.WriteString("\n\n")
		sb.WriteString(styleMuted.Render("  Press enter or q to exit."))
	}

	// Log tail
	if len(m.logs) > 0 && m.step != stepWelcome {
		sb.WriteString("\n\n")
		sb.WriteString(styleMuted.Render("  ─ log ─"))
		sb.WriteString("\n")
		for _, l := range m.logs {
			sb.WriteString("  " + l + "\n")
		}
	}

	// Footer
	footer := styleFooter.Render("  orchard init  ·  ctrl+c quit")
	pad := m.width - lipgloss.Width(footer)
	if pad > 0 {
		footer += strings.Repeat(" ", pad)
	}
	sb.WriteString("\n" + footer)

	return sb.String()
}

// writeConfig writes config.toml with the user's entered values.
func (m Model) writeConfig() error {
	var sb strings.Builder
	sb.WriteString("# orchard configuration\n")
	sb.WriteString("# generated by orchard init\n\n")

	if v := m.fields[0].value; v != "" {
		sb.WriteString(fmt.Sprintf("budget = %s\n", v))
	}
	if v := m.fields[1].value; v != "" {
		sb.WriteString(fmt.Sprintf("watchdog_minutes = %s\n", v))
	}
	if v := m.fields[2].value; v != "" {
		sb.WriteString(fmt.Sprintf("cost_alert = %s\n", v))
	}

	if sb.String() == "# orchard configuration\n# generated by orchard init\n\n" {
		// Nothing entered — skip writing an empty file.
		return nil
	}

	return writeFile(m.configPath, []byte(sb.String()))
}

// Run starts the wizard TUI and returns when the user finishes or quits.
func Run(port int, settingsPath string, configPath string) (Result, error) {
	m := NewModel(port, settingsPath, configPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	fm := finalModel.(Model)
	return Result{
		Completed:  fm.step == stepDone,
		ConfigPath: fm.configPath,
	}, nil
}
