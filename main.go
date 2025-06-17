package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type column struct {
	title   string
	items   []string
	cursor  int
	width   int
	height  int
	focused bool
}

type model struct {
	columns      []column
	activeColumn int
	width        int
	height       int
	odata        *ODataService
	loading      bool
	logs         []string
	showLogs     bool
	serviceIndex int
}

func initialModel() model {
	// Start with service selection
	firstColumn := column{
		title:   "OData Services",
		items:   GetServiceNames(),
		cursor:  0,
		focused: true,
	}
	
	return model{
		columns:      []column{firstColumn},
		activeColumn: 0,
		loading:      false,
		logs:         []string{"Application started"},
		showLogs:     true,
		serviceIndex: -1,
	}
}

type entitySetsMsg []string
type entitiesMsg struct {
	entitySet string
	entities  []map[string]interface{}
}
type errorMsg struct {
	err     string
	context string
}

func (m model) Init() tea.Cmd {
	return nil
}

func loadEntitySets(odata *ODataService) tea.Cmd {
	return func() tea.Msg {
		entitySets, err := odata.GetEntitySets()
		if err != nil {
			return errorMsg{err: err.Error(), context: "loadEntitySets"}
		}
		return entitySetsMsg(entitySets)
	}
}

func loadEntities(odata *ODataService, entitySet string) tea.Cmd {
	return func() tea.Msg {
		entities, err := odata.GetEntities(entitySet, 50)
		if err != nil {
			return errorMsg{err: err.Error(), context: fmt.Sprintf("loadEntities(%s)", entitySet)}
		}
		return entitiesMsg{entitySet: entitySet, entities: entities}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case entitySetsMsg:
		m.loading = false
		m.logs = append(m.logs, fmt.Sprintf("Loaded %d entity sets", len(msg)))
		
		// Find the EntitySets column and update it
		for i := range m.columns {
			if m.columns[i].title == "EntitySets" {
				m.columns[i].items = []string(msg)
				if len(m.columns[i].items) == 0 {
					m.columns[i].items = []string{"(No entity sets)"}
				}
				break
			}
		}

	case entitiesMsg:
		m.loading = false
		m.logs = append(m.logs, fmt.Sprintf("Loaded %d entities from %s", len(msg.entities), msg.entitySet))
		
		// Find the column with matching title
		for i := range m.columns {
			if m.columns[i].title == msg.entitySet {
				m.columns[i].items = []string{}
				for _, entity := range msg.entities {
					m.columns[i].items = append(m.columns[i].items, formatEntityForDisplay(entity))
				}
				if len(m.columns[i].items) == 0 {
					m.columns[i].items = []string{"(No items)"}
				}
				break
			}
		}

	case errorMsg:
		m.loading = false
		m.logs = append(m.logs, fmt.Sprintf("ERROR [%s]: %s", msg.context, msg.err))
		// Keep only last 100 log entries
		if len(m.logs) > 100 {
			m.logs = m.logs[len(m.logs)-100:]
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateColumnSizes()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "f10":
			return m, tea.Quit

		case "up", "k":
			if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				if col.cursor > 0 {
					col.cursor--
				}
			}

		case "down", "j":
			if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				if col.cursor < len(col.items)-1 {
					col.cursor++
				}
			}

		case "right", "l", "enter":
			return m.drillDown()

		case "left", "h", "esc":
			return m.goBack(), nil

		case "f2":
			// TODO: Create entity
		case "f3":
			// TODO: Read entity details
		case "f4":
			// TODO: Update entity
		case "f7":
			// TODO: Filter
		case "f8":
			// TODO: Delete entity
		case "f9":
			m.showLogs = !m.showLogs
		}
	}

	return m, nil
}

func (m *model) updateColumnSizes() {
	if len(m.columns) == 0 {
		return
	}

	totalWidth := m.width
	numColumns := len(m.columns)
	
	// Dynamic width allocation: give more space to active and recent columns
	if numColumns == 1 {
		m.columns[0].width = totalWidth
	} else if numColumns == 2 {
		// 40% for first, 60% for second
		m.columns[0].width = int(float64(totalWidth) * 0.4)
		m.columns[1].width = totalWidth - m.columns[0].width
	} else {
		// For 3+ columns: earlier columns get progressively smaller
		// Active column gets 40%, previous gets 30%, others share the rest
		
		for i := 0; i < numColumns; i++ {
			if i == m.activeColumn {
				m.columns[i].width = int(float64(totalWidth) * 0.4)
			} else if i == m.activeColumn-1 {
				m.columns[i].width = int(float64(totalWidth) * 0.3)
			} else {
				// Other columns share remaining space
				otherCount := numColumns - 2
				if m.activeColumn == 0 {
					otherCount = numColumns - 1
				}
				m.columns[i].width = int(float64(totalWidth) * 0.3 / float64(otherCount))
			}
			
			// Ensure minimum width
			if m.columns[i].width < 20 {
				m.columns[i].width = 20
			}
		}
	}
	
	for i := range m.columns {
		m.columns[i].height = m.height - 4 // Leave space for header and footer
	}
}

func (m model) drillDown() (tea.Model, tea.Cmd) {
	if m.activeColumn >= len(m.columns) {
		return m, nil
	}

	currentCol := m.columns[m.activeColumn]
	if currentCol.cursor >= len(currentCol.items) {
		return m, nil
	}

	selectedItem := currentCol.items[currentCol.cursor]
	
	// Clear focus from current column
	for i := range m.columns {
		m.columns[i].focused = false
	}

	// Add new column or replace existing ones to the right
	if m.activeColumn+1 < len(m.columns) {
		m.columns = m.columns[:m.activeColumn+1]
	}
	
	var newColumn column
	var cmd tea.Cmd
	
	switch m.activeColumn {
	case 0: // Service selection
		// Find selected service
		for i, svc := range DefaultServices {
			if svc.Name == selectedItem {
				m.serviceIndex = i
				m.odata = NewODataServiceWithURL(svc.URL)
				m.logs = append(m.logs, fmt.Sprintf("Connected to %s", svc.Name))
				break
			}
		}
		
		newColumn = column{
			title:   "EntitySets",
			items:   []string{"Loading..."},
			cursor:  0,
			focused: false,
		}
		m.columns = append(m.columns, newColumn)
		m.activeColumn++
		m.columns[m.activeColumn].focused = true
		m.updateColumnSizes()
		m.loading = true
		cmd = loadEntitySets(m.odata)
		
	case 1: // EntitySets -> Entities
		newColumn = column{
			title:   selectedItem,
			items:   []string{"Loading..."},
			cursor:  0,
			focused: false,
		}
		m.columns = append(m.columns, newColumn)
		m.activeColumn++
		m.columns[m.activeColumn].focused = true
		m.updateColumnSizes()
		m.loading = true
		cmd = loadEntities(m.odata, selectedItem)
		
	default: // Entity -> Properties/Details (mock for now)
		newColumn = column{
			title: "Details",
			items: []string{
				"Property 1: Value",
				"Property 2: Value", 
				"Property 3: Value",
			},
			cursor: 0,
			focused: false,
		}
		m.columns = append(m.columns, newColumn)
		m.activeColumn++
		m.columns[m.activeColumn].focused = true
		m.updateColumnSizes()
	}
	
	return m, cmd
}

func (m model) goBack() model {
	if m.activeColumn > 0 {
		// Remove columns to the right of the previous one
		m.columns = m.columns[:m.activeColumn]
		m.activeColumn--
		
		// Focus the previous column
		for i := range m.columns {
			m.columns[i].focused = i == m.activeColumn
		}
		
		m.updateColumnSizes()
	}
	return m
}


func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	
	if len(m.columns) == 0 {
		return "Loading EntitySets..."
	}

	// Calculate dimensions
	bodyHeight := m.height - 5 // header(1) + spacing(2) + footer(1) + spacing(1)
	logHeight := 0
	
	if m.showLogs {
		logHeight = min(10, bodyHeight/3)
		bodyHeight = bodyHeight - logHeight - 1
	}
	
	// Update column heights
	for i := range m.columns {
		m.columns[i].height = bodyHeight
	}

	var columns []string
	
	for i, col := range m.columns {
		columns = append(columns, m.renderColumn(col, i == m.activeColumn))
	}

	headerText := "OData Navigator"
	if m.serviceIndex >= 0 && m.serviceIndex < len(DefaultServices) {
		headerText = fmt.Sprintf("OData Navigator - %s", DefaultServices[m.serviceIndex].Name)
	}
	headerText += " - Use arrows to navigate, Enter to drill down, 'q' to quit"
	
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render(headerText)

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("F2:Create F3:Read F4:Update F7:Filter F8:Delete F9:Toggle Logs F10:Exit | ESC:Back")

	body := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	
	// Build the complete view
	parts := []string{header, "", body}
	
	if m.showLogs {
		logView := m.renderLogs(logHeight)
		parts = append(parts, logView)
	}
	
	parts = append(parts, "", footer)
	
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderLogs(height int) string {
	logStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(height).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("241"))
	
	// Get last N log entries that fit in the height
	startIdx := 0
	if len(m.logs) > height-2 { // -2 for border
		startIdx = len(m.logs) - (height - 2)
	}
	
	var logLines []string
	for i := startIdx; i < len(m.logs); i++ {
		logLines = append(logLines, m.logs[i])
	}
	
	content := strings.Join(logLines, "\n")
	if m.loading {
		content += "\n[Loading...]"
	}
	
	return logStyle.Render(content)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m model) renderColumn(col column, isActive bool) string {
	var items []string
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)
	
	if isActive {
		titleStyle = titleStyle.Foreground(lipgloss.Color("99"))
	} else {
		titleStyle = titleStyle.Foreground(lipgloss.Color("241"))
	}

	for i, item := range col.items {
		style := lipgloss.NewStyle().Padding(0, 1)
		
		if i == col.cursor && isActive {
			style = style.Background(lipgloss.Color("99")).Foreground(lipgloss.Color("0"))
		} else if i == col.cursor {
			style = style.Background(lipgloss.Color("241")).Foreground(lipgloss.Color("15"))
		}
		
		items = append(items, style.Render(item))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, items...)
	
	columnStyle := lipgloss.NewStyle().
		Width(col.width).
		Height(col.height).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("241"))
	
	if isActive {
		columnStyle = columnStyle.BorderForeground(lipgloss.Color("99"))
	}

	return columnStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(col.title),
			"",
			content,
		),
	)
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}