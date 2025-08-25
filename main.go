package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Session struct {
	Name     string
	Windows  int
	Created  string
	Attached bool
}

type Pane struct {
	ID           int    `json:"id"`
	Command      string `json:"command"`
	Position     string `json:"position"`      // "main", "left", "right", "up", "down"
	Parent       int    `json:"parent"`        // ID of parent pane
	SplitPercent int    `json:"split_percent"` // percentage for split (default 50)
	Row          int    `json:"row"`           // Visual row position
	Col          int    `json:"col"`           // Visual column position
	Width        int    `json:"width"`         // Visual width
	Height       int    `json:"height"`        // Visual height
}

type SessionTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"` // Made optional
	Panes       []Pane `json:"panes"`
}

type mode int

const (
	layoutGridW      = 100
	layoutGridH      = 100
	browsing    mode = iota
	creating
	renaming
	confirming
	templateBrowsing
	templateCreating
	templateEditing
	paneEditing
)

type action int

const (
	actionNone action = iota
	actionDelete
	actionKillAll
	actionDeleteTemplate
)

type tickMsg time.Time
type refreshMsg struct{}
type animationTickMsg time.Time

type model struct {
	sessions         []Session
	templates        []SessionTemplate
	cursor           int
	templateCursor   int
	paneCursor       int
	mode             mode
	input            textinput.Model
	commandInput     textinput.Model
	descriptionInput textinput.Model
	message          string
	messageType      string
	width            int
	height           int
	showHelp         bool
	confirmAction    action
	confirmTarget    string
	lastRefresh      time.Time
	autoRefresh      bool
	animationTime    float64
	startTime        time.Time
	lastCursor       int
	popAnimation     float64
	currentTemplate  SessionTemplate
	editingPaneID    int
	showTemplates    bool
	previewMode      bool
}

var terminalCmd string
var (
	primaryColor   = lipgloss.Color("1e66f5")
	secondaryColor = lipgloss.Color("178299")
	accentColor    = lipgloss.Color("e64553")
	warningColor   = lipgloss.Color("df8e1d")
	dangerColor    = lipgloss.Color("fe640b")
	mutedColor     = lipgloss.Color("7287fd")
	successColor   = lipgloss.Color("40a02b")
	templateColor  = lipgloss.Color("8839ef")

	baseStyle = lipgloss.NewStyle().Padding(1, 2)

	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("16")).
				Bold(true).
				Padding(0, 2).
				Align(lipgloss.Center).
				Border(lipgloss.RoundedBorder()).
				BorderBottom(true).
				BorderForeground(primaryColor)

	templateHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("16")).
				Bold(true).
				Padding(0, 2).
				Align(lipgloss.Center).
				Border(lipgloss.RoundedBorder()).
				BorderBottom(true).
				BorderForeground(templateColor)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("16")).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor)

	selectedTemplateStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("16")).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(templateColor)

	attachedIndicator = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true).
				Render("●")

	detachedIndicator = lipgloss.NewStyle().
				Foreground(mutedColor).
				Render("○")

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondaryColor).
			Padding(1, 2).
			Margin(1, 0).
			Width(60).
			Foreground(lipgloss.Color("16"))

	previewBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(templateColor).
			Padding(1, 2).
			Foreground(lipgloss.Color("16"))

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(mutedColor).
			Padding(0, 1)

	selectedPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(accentColor).
				Padding(0, 1)

	infoMessageStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 2)

	successMessageStyle = lipgloss.NewStyle().
				Foreground(successColor).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(successColor).
				Padding(0, 2)

	warningMessageStyle = lipgloss.NewStyle().
				Foreground(warningColor).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(warningColor).
				Padding(0, 2)

	errorMessageStyle = lipgloss.NewStyle().
				Foreground(dangerColor).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(dangerColor).
				Padding(0, 2)

	confirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dangerColor).
			Padding(2, 3).
			Foreground(dangerColor).
			Bold(true)
)

func getDefaultTerminal() string {
	// Check environment variable first
	if term := os.Getenv("LAYTMUX_TERMINAL"); term != "" {
		return term
	}

	// Check common environment variables
	if term := os.Getenv("TERMINAL"); term != "" {
		return term
	}

	// Try to detect available terminals in order of preference
	terminals := []string{"kitty", "alacritty", "gnome-terminal", "xterm", "konsole", "terminator", "tilix"}

	for _, term := range terminals {
		if _, err := exec.LookPath(term); err == nil {
			return term
		}
	}

	// Final fallback
	return "xterm"
}

func validateTerminal(terminal string) error {
	_, err := exec.LookPath(terminal)
	if err != nil {
		return fmt.Errorf("terminal '%s' not found in PATH", terminal)
	}
	return nil
}

// Function to get terminal-specific arguments
func getTerminalArgs(terminal string) []string {
	switch terminal {
	case "kitty":
		return []string{"-e", "tmux", "attach-session", "-t"}
	case "alacritty":
		return []string{"-e", "tmux", "attach-session", "-t"}
	case "gnome-terminal":
		return []string{"--", "tmux", "attach-session", "-t"}
	case "xterm":
		return []string{"-e", "tmux", "attach-session", "-t"}
	case "konsole":
		return []string{"-e", "tmux", "attach-session", "-t"}
	case "terminator":
		return []string{"-e", "tmux", "attach-session", "-t"}
	case "tilix":
		return []string{"-e", "tmux", "attach-session", "-t"}
	default:
		// Default to -e flag for unknown terminals
		return []string{"-e", "tmux", "attach-session", "-t"}
	}
}

func getConfigDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "lazytmux")
}

func getTemplatesFile() string {
	return filepath.Join(getConfigDir(), "templates.json")
}

func loadTemplates() []SessionTemplate {
	templatesFile := getTemplatesFile()
	if _, err := os.Stat(templatesFile); os.IsNotExist(err) {
		return []SessionTemplate{}
	}

	data, err := ioutil.ReadFile(templatesFile)
	if err != nil {
		return []SessionTemplate{}
	}

	var templates []SessionTemplate
	json.Unmarshal(data, &templates)
	return templates
}

func saveTemplates(templates []SessionTemplate) error {
	configDir := getConfigDir()
	os.MkdirAll(configDir, 0755)

	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(getTemplatesFile(), data, 0644)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func animationTick() tea.Cmd {
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return animationTickMsg(t)
	})
}

func refresh() tea.Cmd {
	return func() tea.Msg {
		return refreshMsg{}
	}
}

func listTmuxSessions() []Session {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#S:#{session_windows}:#{session_created}:#{session_attached}").Output()
	if err != nil {
		return []Session{}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	sessions := []Session{}
	for _, line := range lines {
		if line != "" {
			parts := strings.Split(line, ":")
			if len(parts) >= 4 {
				windows := 1
				if w, err := strconv.Atoi(parts[1]); err == nil {
					windows = w
				}

				created := "unknown"
				if ts, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
					created = time.Unix(ts, 0).Format("15:04 02/01")
				}

				attached := parts[3] == "1"

				sessions = append(sessions, Session{
					Name:     parts[0],
					Windows:  windows,
					Created:  created,
					Attached: attached,
				})
			}
		}
	}
	return sessions
}

func generateNumericName(existing []Session) string {
	names := map[int]bool{}
	for _, s := range existing {
		if n, err := strconv.Atoi(s.Name); err == nil {
			names[n] = true
		}
	}
	for i := 0; i <= 999; i++ {
		if !names[i] {
			return strconv.Itoa(i)
		}
	}
	return "0"
}

// Check if a name already exists in sessions or templates
func nameExists(name string, sessions []Session, templates []SessionTemplate) bool {
	for _, s := range sessions {
		if s.Name == name {
			return true
		}
	}
	for _, t := range templates {
		if t.Name == name {
			return true
		}
	}
	return false
}

// Find template by name prefix
func findTemplateByPrefix(name string, templates []SessionTemplate) *SessionTemplate {
	for _, t := range templates {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

func attachSession(name string) {
	args := getTerminalArgs(terminalCmd)
	args = append(args, name)

	cmd := exec.Command(terminalCmd, args...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to launch terminal: %v\n", err)
	}
}

func killSession(name string) error {
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

func killAllSessions() error {
	return exec.Command("tmux", "kill-server").Run()
}

func renameSession(old, new string) error {
	return exec.Command("tmux", "rename-session", "-t", old, new).Run()
}

func createSession(name string) error {
	return exec.Command("tmux", "new-session", "-ds", name).Run()
}

func createSessionFromTemplate(sessionName string, template SessionTemplate) error {
	// Create base session
	if err := createSession(sessionName); err != nil {
		return err
	}

	if len(template.Panes) == 0 {
		return nil
	}

	// Lookup initial (only) pane id
	out, err := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_id}").Output()
	if err != nil {
		return err
	}
	baseID := strings.TrimSpace(string(out))
	idMap := map[int]string{}
	idMap[template.Panes[0].ID] = baseID

	// Command for first pane
	if cmd := strings.TrimSpace(template.Panes[0].Command); cmd != "" {
		_ = exec.Command("tmux", "send-keys", "-t", baseID, cmd, "C-m").Run()
	}

	// Create others in the given order, always selecting parent before split
	for i := 1; i < len(template.Panes); i++ {
		p := template.Panes[i]
		parentID, ok := idMap[p.Parent]
		if !ok {
			// Fallback: split the first pane
			parentID = baseID
		}

		args := []string{"split-window", "-t", parentID}
		switch p.Position {
		case "left", "right":
			args = append(args, "-h")
			if p.Position == "left" {
				args = append(args, "-b") // place on the left of parent
			}
		case "up", "down":
			args = append(args, "-v")
			if p.Position == "up" {
				args = append(args, "-b") // place above parent
			}
		default:
			// default to vertical split
			args = append(args, "-h")
		}

		if p.SplitPercent > 0 && p.SplitPercent != 50 {
			args = append(args, "-p", strconv.Itoa(p.SplitPercent))
		}

		// Print new pane id
		args = append(args, "-P", "-F", "#{pane_id}")

		newOut, err := exec.Command("tmux", args...).Output()
		if err != nil {
			return err
		}
		newID := strings.TrimSpace(string(newOut))
		idMap[p.ID] = newID

		if cmd := strings.TrimSpace(p.Command); cmd != "" {
			_ = exec.Command("tmux", "send-keys", "-t", newID, cmd, "C-m").Run()
		}
	}

	// Focus original pane
	_ = exec.Command("tmux", "select-pane", "-t", baseID).Run()
	return nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tick(), animationTick())
}

func (m *model) setMessage(msg, msgType string) {
	m.message = msg
	m.messageType = msgType
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Calculate visual layout positions for panes
func (m *model) calculatePaneLayout() {
	if len(m.currentTemplate.Panes) == 0 {
		return
	}
	for i := range m.currentTemplate.Panes {
		p := &m.currentTemplate.Panes[i]

		if p.SplitPercent <= 0 {
			p.SplitPercent = 50
		}
		if p.Width < 1 {
			p.Width = 1
		}
		if p.Height < 1 {
			p.Height = 1
		}
		if p.Col < 0 {
			p.Col = 0
		}
		if p.Row < 0 {
			p.Row = 0
		}
		if p.Col+p.Width > layoutGridW {
			p.Width = layoutGridW - p.Col
		}
		if p.Row+p.Height > layoutGridH {
			p.Height = layoutGridH - p.Row
		}
	}
}

func (m *model) findPaneIndex(id int) int {
	for i, pane := range m.currentTemplate.Panes {
		if pane.ID == id {
			return i
		}
	}
	return -1
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case animationTickMsg:
		m.animationTime = time.Since(m.startTime).Seconds()
		if m.popAnimation > 0 {
			m.popAnimation -= 0.05
			if m.popAnimation < 0 {
				m.popAnimation = 0
			}
		}
		cmds = append(cmds, animationTick())

	case tickMsg:
		if m.autoRefresh && time.Since(m.lastRefresh) > 5*time.Second {
			m.sessions = listTmuxSessions()
			m.lastRefresh = time.Now()
		}
		cmds = append(cmds, tick())

	case refreshMsg:
		m.sessions = listTmuxSessions()
		m.templates = loadTemplates()
		m.lastRefresh = time.Now()
		m.setMessage("Sessions and templates refreshed", "success")

	case tea.KeyMsg:
		if m.message != "" {
			m.message = ""
		}

		switch m.mode {
		case browsing:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.lastCursor = m.cursor
					m.cursor--
					m.popAnimation = 0.5
				}
			case "down", "j":
				if m.cursor < len(m.sessions)-1 {
					m.lastCursor = m.cursor
					m.cursor++
					m.popAnimation = 0.5
				}
			case "g":
				if m.cursor != 0 {
					m.lastCursor = m.cursor
					m.cursor = 0
					m.popAnimation = 0.5
				}
			case "G":
				if len(m.sessions) > 0 && m.cursor != len(m.sessions)-1 {
					m.lastCursor = m.cursor
					m.cursor = len(m.sessions) - 1
					m.popAnimation = 0.5
				}
			case "enter", " ":
				if len(m.sessions) > 0 {
					attachSession(m.sessions[m.cursor].Name)
					return m, tea.Quit
				}
			case "n", "c":
				ti := textinput.New()
				ti.Placeholder = "Enter session name (empty for auto-number)"
				ti.Focus()
				ti.CharLimit = 50
				m.input = ti
				m.mode = creating
			case "r":
				if len(m.sessions) > 0 {
					ti := textinput.New()
					ti.Placeholder = "Enter new session name"
					ti.SetValue(m.sessions[m.cursor].Name)
					ti.Focus()
					ti.CharLimit = 50
					m.input = ti
					m.mode = renaming
				}
			case "d":
				if len(m.sessions) > 0 {
					m.confirmAction = actionDelete
					m.confirmTarget = m.sessions[m.cursor].Name
					m.mode = confirming
				}
			case "D":
				if len(m.sessions) > 0 {
					m.confirmAction = actionKillAll
					m.confirmTarget = ""
					m.mode = confirming
				}
			case "ctrl+r", "F5":
				cmds = append(cmds, refresh())
			case "a":
				m.autoRefresh = !m.autoRefresh
				if m.autoRefresh {
					m.setMessage("Auto-refresh enabled", "success")
				} else {
					m.setMessage("Auto-refresh disabled", "info")
				}
			case "t":
				m.showTemplates = true
				m.templateCursor = 0
				m.mode = templateBrowsing
			case "?", "h":
				m.showHelp = !m.showHelp
			}

		case templateBrowsing:
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.showTemplates = false
				m.mode = browsing
			case "up", "k":
				if m.templateCursor > 0 {
					m.templateCursor--
					m.popAnimation = 0.5
				}
			case "down", "j":
				if m.templateCursor < len(m.templates)-1 {
					m.templateCursor++
					m.popAnimation = 0.5
				}
			case "enter", " ":
				if len(m.templates) > 0 {
					// Create session from template
					template := m.templates[m.templateCursor]
					sessionName := fmt.Sprintf("%s-%d", template.Name, time.Now().Unix())

					if err := createSessionFromTemplate(sessionName, template); err != nil {
						m.setMessage(fmt.Sprintf("Failed to create session from template: %v", err), "error")
					} else {
						m.setMessage(fmt.Sprintf("Created session '%s' from template '%s'", sessionName, template.Name), "success")
						attachSession(sessionName)
						return m, tea.Quit
					}
				}
			case "n", "c":
				// Create new template
				m.currentTemplate = SessionTemplate{
					Name:        "",
					Description: "",
					Panes: []Pane{{
						ID:           1,
						Command:      "",
						Position:     "main",
						Parent:       0,
						SplitPercent: 50,
						Row:          0,
						Col:          0,
						Width:        layoutGridW,
						Height:       layoutGridH,
					}},
				}
				m.editingPaneID = 1

				ti := textinput.New()
				ti.Placeholder = "Enter template name"
				ti.Focus()
				ti.CharLimit = 50
				m.input = ti

				desc := textinput.New()
				desc.Placeholder = "Enter template description (optional)"
				desc.CharLimit = 100
				m.descriptionInput = desc

				m.mode = templateCreating
			case "e":
				if len(m.templates) > 0 {
					m.currentTemplate = m.templates[m.templateCursor]
					m.editingPaneID = 1
					m.paneCursor = 0
					m.calculatePaneLayout()
					m.mode = templateEditing
				}
			case "d":
				if len(m.templates) > 0 {
					m.confirmAction = actionDeleteTemplate
					m.confirmTarget = m.templates[m.templateCursor].Name
					m.mode = confirming
				}
			case "p":
				m.previewMode = !m.previewMode
			case "?", "h":
				m.showHelp = !m.showHelp
			}

		case templateCreating:
			var cmd tea.Cmd
			if m.input.Focused() {
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				m.descriptionInput, cmd = m.descriptionInput.Update(msg)
				cmds = append(cmds, cmd)
			}

			switch msg.String() {
			case "tab":
				if m.input.Focused() {
					m.input.Blur()
					m.descriptionInput.Focus()
				} else {
					m.descriptionInput.Blur()
					m.input.Focus()
				}
			case "enter":
				if m.input.Focused() {
					name := strings.TrimSpace(m.input.Value())
					if name == "" {
						m.setMessage("Template name cannot be empty", "error")
						break
					}

					// Check for duplicate names
					if nameExists(name, m.sessions, m.templates) {
						m.setMessage("Name already exists", "error")
						break
					}

					m.input.Blur()
					m.descriptionInput.Focus()
				} else {
					// Save template
					name := strings.TrimSpace(m.input.Value())
					if name == "" {
						m.setMessage("Template name cannot be empty", "error")
						break
					}

					// Check for duplicate names again
					if nameExists(name, m.sessions, m.templates) {
						m.setMessage("Name already exists", "error")
						break
					}

					m.currentTemplate.Name = name
					m.currentTemplate.Description = strings.TrimSpace(m.descriptionInput.Value())
					m.calculatePaneLayout()

					m.templates = append(m.templates, m.currentTemplate)
					if err := saveTemplates(m.templates); err != nil {
						m.setMessage(fmt.Sprintf("Failed to save template: %v", err), "error")
					} else {
						m.setMessage(fmt.Sprintf("Template '%s' created", m.currentTemplate.Name), "success")
						m.mode = templateBrowsing
						m.templateCursor = len(m.templates) - 1
					}
				}
			case "esc":
				m.mode = templateBrowsing
				m.input.SetValue("")
				m.descriptionInput.SetValue("")
			}

		case templateEditing:
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.mode = templateBrowsing
			case "up", "k":
				if m.paneCursor > 0 {
					m.paneCursor--
				}
			case "down", "j":
				if m.paneCursor < len(m.currentTemplate.Panes)-1 {
					m.paneCursor++
				}
			case "enter", "e":
				if len(m.currentTemplate.Panes) > 0 {
					m.editingPaneID = m.currentTemplate.Panes[m.paneCursor].ID

					cmd := textinput.New()
					cmd.Placeholder = "Enter command for pane"
					cmd.SetValue(m.currentTemplate.Panes[m.paneCursor].Command)
					cmd.Focus()
					cmd.CharLimit = 100
					m.commandInput = cmd

					m.mode = paneEditing
				}
			case "H":
				m.addPane("left")
			case "L":
				m.addPane("right")
			case "J":
				m.addPane("down")
			case "K":
				m.addPane("up")
			case "d":
				if len(m.currentTemplate.Panes) > 1 && m.paneCursor < len(m.currentTemplate.Panes) {
					m.currentTemplate.Panes = append(m.currentTemplate.Panes[:m.paneCursor], m.currentTemplate.Panes[m.paneCursor+1:]...)
					if m.paneCursor >= len(m.currentTemplate.Panes) {
						m.paneCursor = len(m.currentTemplate.Panes) - 1
					}
					m.calculatePaneLayout()
				}
			case "s":
				// Save template
				for i, template := range m.templates {
					if template.Name == m.currentTemplate.Name {
						m.templates[i] = m.currentTemplate
						break
					}
				}
				if err := saveTemplates(m.templates); err != nil {
					m.setMessage(fmt.Sprintf("Failed to save template: %v", err), "error")
				} else {
					m.setMessage("Template saved", "success")
					m.mode = templateBrowsing
				}
			}

		case paneEditing:
			var cmd tea.Cmd
			m.commandInput, cmd = m.commandInput.Update(msg)
			cmds = append(cmds, cmd)

			switch msg.String() {
			case "enter":
				// Update pane command
				for i := range m.currentTemplate.Panes {
					if m.currentTemplate.Panes[i].ID == m.editingPaneID {
						m.currentTemplate.Panes[i].Command = strings.TrimSpace(m.commandInput.Value())
						break
					}
				}
				m.mode = templateEditing
			case "esc":
				m.mode = templateEditing
			}

		case creating, renaming:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)

			switch msg.String() {
			case "enter":
				val := strings.TrimSpace(m.input.Value())
				if m.mode == creating {
					if val == "" {
						val = generateNumericName(m.sessions)
					}

					// Check if session name matches a template prefix
					template := findTemplateByPrefix(val, m.templates)
					if template != nil {
						// Create session from template
						if err := createSessionFromTemplate(val, *template); err != nil {
							m.setMessage(fmt.Sprintf("Failed to create session from template: %v", err), "error")
						} else {
							m.setMessage(fmt.Sprintf("Created session '%s' from template '%s'", val, template.Name), "success")
							attachSession(val)
							return m, tea.Quit
						}
					} else {
						// Create regular session
						if err := createSession(val); err != nil {
							m.setMessage(fmt.Sprintf("Failed to create session: %v", err), "error")
						} else {
							m.setMessage(fmt.Sprintf("Created session '%s'", val), "success")
							attachSession(val)
							return m, tea.Quit
						}
					}
				} else if m.mode == renaming && val != "" {
					// Check for duplicate names
					if nameExists(val, m.sessions, m.templates) {
						m.setMessage("Name already exists", "error")
						break
					}

					oldName := m.sessions[m.cursor].Name
					if err := renameSession(oldName, val); err != nil {
						m.setMessage(fmt.Sprintf("Failed to rename session: %v", err), "error")
					} else {
						m.setMessage(fmt.Sprintf("Renamed '%s' to '%s'", oldName, val), "success")
					}
				}
				m.sessions = listTmuxSessions()
				m.mode = browsing
				m.input.SetValue("")

			case "esc":
				m.mode = browsing
				m.input.SetValue("")
			}

		case confirming:
			switch msg.String() {
			case "y", "enter":
				switch m.confirmAction {
				case actionDelete:
					if err := killSession(m.confirmTarget); err != nil {
						m.setMessage(fmt.Sprintf("Failed to delete session: %v", err), "error")
					} else {
						m.setMessage(fmt.Sprintf("Deleted session '%s'", m.confirmTarget), "success")
					}
				case actionKillAll:
					if err := killAllSessions(); err != nil {
						m.setMessage(fmt.Sprintf("Failed to kill all sessions: %v", err), "error")
					} else {
						m.setMessage("All sessions killed", "warning")
					}
				case actionDeleteTemplate:
					for i, template := range m.templates {
						if template.Name == m.confirmTarget {
							m.templates = append(m.templates[:i], m.templates[i+1:]...)
							break
						}
					}
					if err := saveTemplates(m.templates); err != nil {
						m.setMessage(fmt.Sprintf("Failed to delete template: %v", err), "error")
					} else {
						m.setMessage(fmt.Sprintf("Deleted template '%s'", m.confirmTarget), "success")
					}
					if m.templateCursor >= len(m.templates) && len(m.templates) > 0 {
						m.templateCursor = len(m.templates) - 1
					}
				}
				m.sessions = listTmuxSessions()
				if m.cursor >= len(m.sessions) && len(m.sessions) > 0 {
					m.cursor = len(m.sessions) - 1
				} else if len(m.sessions) == 0 {
					m.cursor = 0
				}
				if m.showTemplates {
					m.mode = templateBrowsing
				} else {
					m.mode = browsing
				}

			case "n", "esc":
				if m.showTemplates {
					m.mode = templateBrowsing
				} else {
					m.mode = browsing
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *model) addPane(direction string) {
	if len(m.currentTemplate.Panes) == 0 {
		return
	}

	// Find selected pane
	if m.paneCursor < 0 || m.paneCursor >= len(m.currentTemplate.Panes) {
		m.paneCursor = 0
	}
	sel := &m.currentTemplate.Panes[m.paneCursor]

	// Next unique ID
	newID := 1
	for _, p := range m.currentTemplate.Panes {
		if p.ID >= newID {
			newID = p.ID + 1
		}
	}

	split := sel.SplitPercent
	if split <= 0 {
		split = 50
	}

	newPane := Pane{
		ID:           newID,
		Command:      "",
		Position:     direction,
		Parent:       sel.ID,
		SplitPercent: split,
	}

	switch direction {
	case "left":
		// vertical split; new pane occupies left portion
		newW := sel.Width * split / 100
		if newW < 1 {
			newW = 1
		}
		rem := sel.Width - newW
		if rem < 1 {
			rem = 1
			if newW > 1 {
				newW--
			}
		}
		newPane.Row = sel.Row
		newPane.Col = sel.Col
		newPane.Width = newW
		newPane.Height = sel.Height

		sel.Col = sel.Col + newW
		sel.Width = rem

	case "right":
		// vertical split; new pane occupies right portion
		newW := sel.Width * split / 100
		if newW < 1 {
			newW = 1
		}
		rem := sel.Width - newW
		if rem < 1 {
			rem = 1
			if newW > 1 {
				newW--
			}
		}
		newPane.Row = sel.Row
		newPane.Col = sel.Col + rem
		newPane.Width = newW
		newPane.Height = sel.Height

		sel.Width = rem

	case "up":
		// horizontal split; new pane occupies upper portion
		newH := sel.Height * split / 100
		if newH < 1 {
			newH = 1
		}
		rem := sel.Height - newH
		if rem < 1 {
			rem = 1
			if newH > 1 {
				newH--
			}
		}
		newPane.Row = sel.Row
		newPane.Col = sel.Col
		newPane.Width = sel.Width
		newPane.Height = newH

		sel.Row = sel.Row + newH
		sel.Height = rem

	case "down":
		// horizontal split; new pane occupies lower portion
		newH := sel.Height * split / 100
		if newH < 1 {
			newH = 1
		}
		rem := sel.Height - newH
		if rem < 1 {
			rem = 1
			if newH > 1 {
				newH--
			}
		}
		newPane.Row = sel.Row + rem
		newPane.Col = sel.Col
		newPane.Width = sel.Width
		newPane.Height = newH

		sel.Height = rem
	}

	m.currentTemplate.Panes = append(m.currentTemplate.Panes, newPane)

	// Optionally focus the newly created pane in the editor
	m.paneCursor = len(m.currentTemplate.Panes) - 1

	// Clamp
	m.calculatePaneLayout()
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var content strings.Builder
	tableWidth := min(m.width-4, 100)

	if m.showTemplates {
		return m.renderTemplateView(tableWidth)
	}

	// Regular session view
	if len(m.sessions) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Render("No tmux sessions found. Press 'n' to create a new session or 't' for templates.")
		content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, emptyMsg))
		content.WriteString("\n\n")
	} else {
		nameHeader := tableHeaderStyle.Width(tableWidth * 2 / 5).Render("SESSION NAME")
		statusHeader := tableHeaderStyle.Width(tableWidth / 6).Render("STATUS")
		windowsHeader := tableHeaderStyle.Width(tableWidth / 6).Render("WINDOWS")
		createdHeader := tableHeaderStyle.Width(tableWidth / 6).Render("CREATED")

		headerRow := lipgloss.JoinHorizontal(lipgloss.Top, nameHeader, statusHeader, windowsHeader, createdHeader)
		content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, headerRow))
		content.WriteString("\n")

		for i, session := range m.sessions {
			isSelected := m.cursor == i && m.mode == browsing

			rowStyle := selectedRowStyle.Copy().Padding(0, 1)

			if isSelected && m.popAnimation > 0 {
				scale := 1.0 + (m.popAnimation * 0.2)
				rowStyle = rowStyle.Copy().MarginLeft(int(scale)).MarginRight(int(scale))
			}

			nameText := "  " + session.Name
			if isSelected {
				nameText = "▶ " + session.Name
			}

			statusText := detachedIndicator + " Detached"
			if session.Attached {
				statusText = attachedIndicator + " Active"
			}

			nameCell := rowStyle.Copy().Width(tableWidth * 2 / 5).Render(nameText)
			statusCell := rowStyle.Copy().Width(tableWidth / 6).Render(statusText)
			windowsCell := rowStyle.Copy().Width(tableWidth / 6).Render(fmt.Sprintf("%d", session.Windows))
			createdCell := rowStyle.Copy().Width(tableWidth / 6).Render(session.Created)

			row := lipgloss.JoinHorizontal(lipgloss.Top, nameCell, statusCell, windowsCell, createdCell)
			content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, row))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	if m.mode == creating || m.mode == renaming {
		var inputPrompt string
		if m.mode == creating {
			inputPrompt = "✨ Create new session:"
		} else {
			inputPrompt = "🔄 Rename session:"
		}
		inputView := inputBoxStyle.Render(fmt.Sprintf("%s\n%s", inputPrompt, m.input.View()))
		content.WriteString(lipgloss.Place(m.width, 4, lipgloss.Center, lipgloss.Top, inputView))
		content.WriteString("\n")
	}

	if m.mode == confirming {
		var confirmText string
		switch m.confirmAction {
		case actionDelete:
			confirmText = fmt.Sprintf("⚠️  DELETE SESSION '%s'?\n\nThis action cannot be undone!\n\n[y] Yes  [n] No", m.confirmTarget)
		case actionKillAll:
			confirmText = fmt.Sprintf("💀 KILL ALL %d SESSIONS?\n\nThis will destroy ALL sessions!\nThis action cannot be undone!\n\n[y] Yes  [n] No", len(m.sessions))
		}
		confirmView := confirmBoxStyle.Render(confirmText)
		content.WriteString(lipgloss.Place(m.width, 7, lipgloss.Center, lipgloss.Center, confirmView))
	}

	if m.message != "" {
		var msgStyle lipgloss.Style
		switch m.messageType {
		case "success":
			msgStyle = successMessageStyle
		case "warning":
			msgStyle = warningMessageStyle
		case "error":
			msgStyle = errorMessageStyle
		default:
			msgStyle = infoMessageStyle
		}
		statusMsg := msgStyle.Render(m.message)
		content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, statusMsg))
		content.WriteString("\n")
	}

	var statusItems []string
	statusItems = append(statusItems, fmt.Sprintf("📊 Sessions: %d", len(m.sessions)))
	statusItems = append(statusItems, fmt.Sprintf("📋 Templates: %d", len(m.templates)))
	if m.autoRefresh {
		statusItems = append(statusItems, "🔄 Auto-refresh: ON")
	}
	statusItems = append(statusItems, "❓ Press ? for help")

	statusBarText := strings.Join(statusItems, " • ")
	statusBar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("16")).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderTop(true).
		BorderForeground(primaryColor).
		Render(statusBarText)
	content.WriteString("\n")
	content.WriteString(lipgloss.Place(tableWidth, 1, lipgloss.Center, lipgloss.Top, statusBar))

	if m.showHelp {
		helpContent := strings.Builder{}
		helpContent.WriteString(lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Underline(true).Padding(0, 1).Render("KEYBOARD SHORTCUTS") + "\n\n")
		shortcuts := [][]string{
			{"↑/k", "Move up"},
			{"↓/j", "Move down"},
			{"g", "Go to top"},
			{"G", "Go to bottom"},
			{"Enter/Space", "Attach to session"},
			{"n/c", "Create new session"},
			{"t", "Browse templates"},
			{"r", "Rename session"},
			{"d", "Delete session"},
			{"D", "Delete ALL sessions"},
			{"Ctrl+R/F5", "Refresh sessions"},
			{"a", "Toggle auto-refresh"},
			{"?/h", "Toggle this help"},
			{"q/Ctrl+C", "Quit"},
		}
		for _, shortcut := range shortcuts {
			key := lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accentColor).
				Render(shortcut[0])
			desc := lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Render(shortcut[1])
			helpContent.WriteString(fmt.Sprintf("%s  %s\n", key, desc))
		}
		helpBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			Width(40).
			Render(helpContent.String())
		content.WriteString(lipgloss.Place(m.width, m.height-10, lipgloss.Right, lipgloss.Top, helpBox))
	}

	return baseStyle.Render(content.String())
}

func (m model) renderTemplateView(tableWidth int) string {
	var content strings.Builder

	// Title
	title := templateHeaderStyle.Width(tableWidth).Render("🚀 SESSION TEMPLATES")
	content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, title))
	content.WriteString("\n\n")

	if len(m.templates) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Render("No templates found. Press 'n' to create your first template.")
		content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, emptyMsg))
		content.WriteString("\n\n")
	} else {
		// Template list
		for i, template := range m.templates {
			isSelected := m.templateCursor == i && (m.mode == templateBrowsing)

			rowStyle := selectedTemplateStyle.Copy().Padding(0, 1)
			if !isSelected {
				rowStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("16")).
					Padding(0, 1).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(mutedColor)
			}

			if isSelected && m.popAnimation > 0 {
				scale := 1.0 + (m.popAnimation * 0.2)
				rowStyle = rowStyle.Copy().MarginLeft(int(scale)).MarginRight(int(scale))
			}

			nameText := template.Name
			if isSelected {
				nameText = "▶ " + template.Name
			} else {
				nameText = "  " + template.Name
			}

			paneCount := fmt.Sprintf("%d panes", len(template.Panes))
			if len(template.Panes) == 1 {
				paneCount = "1 pane"
			}

			description := template.Description
			if len(description) > 40 {
				description = description[:40] + "..."
			}
			if description == "" {
				description = "(no description)"
			}

			nameCell := rowStyle.Copy().Width(tableWidth / 3).Render(nameText)
			paneCell := rowStyle.Copy().Width(tableWidth / 6).Render(paneCount)
			descCell := rowStyle.Copy().Width(tableWidth / 2).Render(description)

			row := lipgloss.JoinHorizontal(lipgloss.Top, nameCell, paneCell, descCell)
			content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, row))
			content.WriteString("\n")
		}
	}

	// Handle different modes
	switch m.mode {
	case templateCreating:
		var inputPrompt string
		inputPrompt = "📝 Create Template\n\nName: " + m.input.View() + "\nDescription: " + m.descriptionInput.View() + "\n\n[Tab] Switch fields • [Enter] Save • [Esc] Cancel"
		inputView := inputBoxStyle.Render(inputPrompt)
		content.WriteString(lipgloss.Place(m.width, 8, lipgloss.Center, lipgloss.Top, inputView))

	case templateEditing:
		editView := m.renderTemplateEditor()
		content.WriteString(editView)

	case paneEditing:
		inputPrompt := fmt.Sprintf("✏️ Edit Pane Command\n\n%s", m.commandInput.View())
		inputView := inputBoxStyle.Render(inputPrompt)
		content.WriteString(lipgloss.Place(m.width, 4, lipgloss.Center, lipgloss.Top, inputView))

	case confirming:
		var confirmText string
		if m.confirmAction == actionDeleteTemplate {
			confirmText = fmt.Sprintf("⚠️  DELETE TEMPLATE '%s'?\n\nThis action cannot be undone!\n\n[y] Yes  [n] No", m.confirmTarget)
		}
		confirmView := confirmBoxStyle.Render(confirmText)
		content.WriteString(lipgloss.Place(m.width, 7, lipgloss.Center, lipgloss.Center, confirmView))
	}

	// Status and help for templates
	if m.message != "" {
		var msgStyle lipgloss.Style
		switch m.messageType {
		case "success":
			msgStyle = successMessageStyle
		case "warning":
			msgStyle = warningMessageStyle
		case "error":
			msgStyle = errorMessageStyle
		default:
			msgStyle = infoMessageStyle
		}
		statusMsg := msgStyle.Render(m.message)
		content.WriteString(lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Top, statusMsg))
		content.WriteString("\n")
	}

	// Template status bar
	var statusItems []string
	statusItems = append(statusItems, fmt.Sprintf("📋 Templates: %d", len(m.templates)))
	if m.previewMode {
		statusItems = append(statusItems, "👁️ Preview: ON")
	}
	statusItems = append(statusItems, "❓ Press ? for help")

	statusBarText := strings.Join(statusItems, " • ")
	statusBar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("16")).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderTop(true).
		BorderForeground(templateColor).
		Render(statusBarText)
	content.WriteString("\n")
	content.WriteString(lipgloss.Place(tableWidth, 1, lipgloss.Center, lipgloss.Top, statusBar))

	if m.showHelp {
		helpContent := strings.Builder{}
		helpContent.WriteString(lipgloss.NewStyle().Foreground(templateColor).Bold(true).Underline(true).Padding(0, 1).Render("TEMPLATE SHORTCUTS") + "\n\n")

		var shortcuts [][]string
		if m.mode == templateBrowsing {
			shortcuts = [][]string{
				{"↑/k", "Move up"},
				{"↓/j", "Move down"},
				{"Enter/Space", "Create session from template"},
				{"n/c", "Create new template"},
				{"e", "Edit template"},
				{"d", "Delete template"},
				{"p", "Toggle preview"},
				{"Esc", "Back to sessions"},
				{"?/h", "Toggle help"},
			}
		} else if m.mode == templateEditing {
			shortcuts = [][]string{
				{"↑/k", "Move up panes"},
				{"↓/j", "Move down panes"},
				{"Enter/e", "Edit pane command"},
				{"H", "Add pane left of selected"},
				{"J", "Add pane down of selected"},
				{"K", "Add pane up of selected"},
				{"L", "Add pane right of selected"},
				{"d", "Delete pane"},
				{"s", "Save template"},
				{"Esc", "Back to templates"},
			}
		}

		for _, shortcut := range shortcuts {
			key := lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accentColor).
				Render(shortcut[0])
			desc := lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Render(shortcut[1])
			helpContent.WriteString(fmt.Sprintf("%s  %s\n", key, desc))
		}

		helpBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(templateColor).
			Padding(1, 2).
			Width(45).
			Render(helpContent.String())
		content.WriteString(lipgloss.Place(m.width, m.height-15, lipgloss.Right, lipgloss.Top, helpBox))
	}

	return baseStyle.Render(content.String())
}

// Replace your existing renderTemplateEditor() with this version.
// It draws a scaled ASCII grid of the template's panes and highlights the selected pane.
func (m model) renderTemplateEditor() string {
	var content strings.Builder

	// Title
	title := fmt.Sprintf("✏️ Editing: %s", m.currentTemplate.Name)
	content.WriteString(lipgloss.NewStyle().
		Foreground(templateColor).
		Bold(true).
		Render(title))
	content.WriteString("\n\n")

	// Determine layout bounds (dynamic - scale to content)
	maxRow, maxCol := 1, 1
	for _, p := range m.currentTemplate.Panes {
		if rr := p.Row + p.Height; rr > maxRow {
			maxRow = rr
		}
		if cc := p.Col + p.Width; cc > maxCol {
			maxCol = cc
		}
	}
	if maxRow <= 0 {
		maxRow = 1
	}
	if maxCol <= 0 {
		maxCol = 1
	}

	// Preview canvas size (characters). Adjust these if you want a bigger/smaller editor.
	const pr = 12 // preview rows (height)
	const pc = 48 // preview cols (width)

	// Initialize the canvas with spaces
	grid := make([][]rune, pr)
	for r := 0; r < pr; r++ {
		grid[r] = make([]rune, pc)
		for c := 0; c < pc; c++ {
			grid[r][c] = ' '
		}
	}

	// Helper to keep coords sane
	clip := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}

	// Draw each pane as a box on the canvas
	for idx, pane := range m.currentTemplate.Panes {
		// Map template coordinates -> preview coordinates (inclusive/exclusive)
		r0 := pane.Row * pr / maxRow
		r1 := (pane.Row + pane.Height) * pr / maxRow
		c0 := pane.Col * pc / maxCol
		c1 := (pane.Col + pane.Width) * pc / maxCol

		// Ensure minimum size to draw a box
		if r1 <= r0+1 {
			r1 = clip(r0+3, 0, pr)
		}
		if c1 <= c0+2 {
			c1 = clip(c0+6, 0, pc)
		}
		if r0 < 0 {
			r0 = 0
		}
		if c0 < 0 {
			c0 = 0
		}
		if r1 > pr {
			r1 = pr
		}
		if c1 > pc {
			c1 = pc
		}

		// Choose border style for selected pane (double lines) vs others (single)
		var (
			hChar, vChar   rune = '─', '│'
			tl, tr, bl, br rune = '┌', '┐', '└', '┘'
		)
		if m.paneCursor >= 0 && m.paneCursor < len(m.currentTemplate.Panes) &&
			m.currentTemplate.Panes[m.paneCursor].ID == pane.ID && m.mode == templateEditing {
			hChar, vChar = '═', '║'
			tl, tr, bl, br = '╔', '╗', '╚', '╝'
		}

		// Top and bottom horizontal lines
		for c := c0 + 1; c < c1-1; c++ {
			if r0 >= 0 && r0 < pr && c >= 0 && c < pc {
				grid[r0][c] = hChar
			}
			if r1-1 >= 0 && r1-1 < pr && c >= 0 && c < pc {
				grid[r1-1][c] = hChar
			}
		}

		// Left and right vertical lines
		for r := r0 + 1; r < r1-1; r++ {
			if c0 >= 0 && c0 < pc && r >= 0 && r < pr {
				grid[r][c0] = vChar
			}
			if c1-1 >= 0 && c1-1 < pc && r >= 0 && r < pr {
				grid[r][c1-1] = vChar
			}
		}

		// Corners
		if r0 >= 0 && r0 < pr && c0 >= 0 && c0 < pc {
			grid[r0][c0] = tl
		}
		if r0 >= 0 && r0 < pr && c1-1 >= 0 && c1-1 < pc {
			grid[r0][c1-1] = tr
		}
		if r1-1 >= 0 && r1-1 < pr && c0 >= 0 && c0 < pc {
			grid[r1-1][c0] = bl
		}
		if r1-1 >= 0 && r1-1 < pr && c1-1 >= 0 && c1-1 < pc {
			grid[r1-1][c1-1] = br
		}

		// Fill interior with spaces (already spaces) and write the command on the first interior line
		cmd := strings.TrimSpace(pane.Command)
		if cmd == "" {
			cmd = "(empty)"
		}
		rText := r0 + 1
		cTextStart := c0 + 1
		maxTextWidth := (c1 - 1) - (c0 + 1) // interior width
		if maxTextWidth < 0 {
			maxTextWidth = 0
		}
		// Truncate command to fit
		cmdRunes := []rune(cmd)
		if len(cmdRunes) > maxTextWidth {
			cmdRunes = cmdRunes[:maxTextWidth]
		}
		for i, rr := range cmdRunes {
			c := cTextStart + i
			if rText >= 0 && rText < pr && c >= 0 && c < pc {
				grid[rText][c] = rr
			}
		}

		// If the pane is selected, add a small marker in its top-right interior (visual cue)
		if m.mode == templateEditing && m.paneCursor < len(m.currentTemplate.Panes) && m.currentTemplate.Panes[m.paneCursor].ID == pane.ID {
			mrkR := r0 + 1
			mrkC := c1 - 3
			if mrkR >= 0 && mrkR < pr && mrkC >= 0 && mrkC < pc {
				grid[mrkR][mrkC] = '●'
			}
		}

		// Prevent accidental overlap garbling: when drawing later panes, they will overwrite earlier characters.
		_ = idx // keep in case you want per-pane behavior later
	}

	// Render the canvas rows into a string
	content.WriteString("\n")
	for r := 0; r < pr; r++ {
		content.WriteString(string(grid[r]))
		content.WriteString("\n")
	}
	content.WriteString("\n")

	// Add editor command hints (compact)
	hints := "Commands: [H/J/K/L] Split selected • [Enter/e] Edit command • [d] Delete pane • [s] Save • [Esc] Back"
	content.WriteString(hints)

	// Use same box style as before for consistency
	return inputBoxStyle.Width(80).Render(content.String())
}

func main() {
	// Define command line flags
	var (
		terminal    = flag.String("t", "", "Terminal emulator to use (e.g., kitty, alacritty, gnome-terminal)")
		showHelp    = flag.Bool("h", false, "Show help")
		showVersion = flag.Bool("v", false, "Show version")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "A modern TUI for managing tmux sessions and templates.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nTerminal Detection:\n")
		fmt.Fprintf(os.Stderr, "  1. Command line flag (-t)\n")
		fmt.Fprintf(os.Stderr, "  2. LAYTMUX_TERMINAL environment variable\n")
		fmt.Fprintf(os.Stderr, "  3. TERMINAL environment variable\n")
		fmt.Fprintf(os.Stderr, "  4. Auto-detect from: kitty, alacritty, gnome-terminal, xterm, konsole, terminator, tilix\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                          # Auto-detect terminal\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -t alacritty             # Use alacritty\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  LAZYTMUX_TERMINAL=kitty %s  # Use environment variable\n", os.Args[0])
	}

	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Println("lazytmux version 0.0.1")
		os.Exit(0)
	}

	// Determine which terminal to use
	if *terminal != "" {
		terminalCmd = *terminal
	} else {
		terminalCmd = getDefaultTerminal()
	}

	// Validate the terminal
	if err := validateTerminal(terminalCmd); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run '%s -h' for help on terminal detection.\n", os.Args[0])

		// Show available terminals
		fmt.Fprintf(os.Stderr, "\nTrying to find available terminals...\n")
		terminals := []string{"kitty", "alacritty", "gnome-terminal", "xterm", "konsole", "terminator", "tilix"}
		found := false
		for _, term := range terminals {
			if _, err := exec.LookPath(term); err == nil {
				fmt.Fprintf(os.Stderr, "  ✓ %s (available)\n", term)
				found = true
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "  No supported terminals found in PATH\n")
		}
		os.Exit(1)
	}

	fmt.Printf("Using terminal: %s\n", terminalCmd)

	sessions := listTmuxSessions()
	templates := loadTemplates()

	m := model{
		sessions:       sessions,
		templates:      templates,
		cursor:         0,
		templateCursor: 0,
		paneCursor:     0,
		mode:           browsing,
		lastRefresh:    time.Now(),
		autoRefresh:    true,
		startTime:      time.Now(),
		lastCursor:     -1,
		popAnimation:   0,
		showTemplates:  false,
		previewMode:    true,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
