package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type column struct {
	title     string
	items     []string
	cursor    int
	scrollOffset int                   // For large content scrolling
	width     int
	height    int
	focused   bool
	entities  []map[string]interface{} // Store actual entity data
	isDetails bool                     // Flag to indicate if this is a details column
	isPreview bool                     // Flag to indicate if this is a preview column
}

type model struct {
	columns        []column
	activeColumn   int
	previewColumn  *column  // Always-present preview column
	width          int
	height         int
	odata          *ODataService
	loading        bool
	logs           []string
	showLogs       bool
	services       []ServiceConfig
	serviceIndex   int
	editMode       bool
	editContent    []string
	editCursor     int     // Current cursor position in edit mode
	previewLoading bool
	modalEditor    bool    // Modal editor mode
	modalContent   []string // Content being edited in modal
	modalCursor    int     // Cursor position in modal
	modalScroll    int     // Scroll offset in modal
}

func initialModel() model {
	// Load configuration
	services := LoadConfig()
	
	// Start with service selection
	firstColumn := column{
		title:   "OData Services",
		items:   GetServiceNames(services),
		cursor:  0,
		focused: true,
	}
	
	// Initialize preview column
	previewCol := &column{
		title:     "Preview",
		items:     []string{"Select a service to preview entity sets"},
		cursor:    0,
		focused:   false,
		isPreview: true,
	}
	
	return model{
		columns:       []column{firstColumn},
		activeColumn:  0,
		previewColumn: previewCol,
		loading:       false,
		logs:          []string{"Application started"},
		showLogs:      true,
		services:      services,
		serviceIndex:  -1,
	}
}

type entitySetsMsg []string
type entitiesMsg struct {
	entitySet string
	entities  []map[string]interface{}
	hasMore   bool
}
type previewMsg struct {
	previewType string // "entitysets", "entities", "json"
	data        interface{}
	errorMsg    string
}
type entityDetailMsg struct {
	entitySet string
	entityKey string
	entity    map[string]interface{}
}
type errorMsg struct {
	err     string
	context string
}

func (m model) Init() tea.Cmd {
	// Trigger initial preview update  
	return m.updatePreview()
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
		entities, hasMore, err := odata.GetEntitiesWithCount(entitySet, 10) // Default to 10 entities
		if err != nil {
			return errorMsg{err: err.Error(), context: fmt.Sprintf("loadEntities(%s)", entitySet)}
		}
		return entitiesMsg{entitySet: entitySet, entities: entities, hasMore: hasMore}
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
				m.columns[i].items = []string{}
				for _, entitySet := range msg {
					capabilities := GetEntitySetCapabilities(entitySet)
					displayText := fmt.Sprintf("%s %s", entitySet, capabilities.String())
					m.columns[i].items = append(m.columns[i].items, displayText)
				}
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
				m.columns[i].entities = msg.entities
				m.columns[i].items = []string{}
				for _, entity := range msg.entities {
					m.columns[i].items = append(m.columns[i].items, formatEntityForDisplay(entity))
				}
				// Add "more" indicator if truncated
				if msg.hasMore {
					m.columns[i].items = append(m.columns[i].items, "[...more items]")
				}
				if len(m.columns[i].items) == 0 {
					m.columns[i].items = []string{"(No items)"}
				}
				break
			}
		}

	case previewMsg:
		m.previewLoading = false
		if m.previewColumn != nil {
			if msg.errorMsg != "" {
				m.previewColumn.items = []string{fmt.Sprintf("Error: %s", msg.errorMsg)}
			} else {
				switch msg.previewType {
				case "entitysets":
					if entitySets, ok := msg.data.([]string); ok {
						m.previewColumn.title = "EntitySets Preview"
						m.previewColumn.items = []string{}
						for _, es := range entitySets {
							caps := GetEntitySetCapabilities(es)
							m.previewColumn.items = append(m.previewColumn.items, fmt.Sprintf("%s %s", es, caps.String()))
						}
					}
				case "entities":
					if entities, ok := msg.data.([]map[string]interface{}); ok {
						m.previewColumn.title = "Entities Preview"
						m.previewColumn.items = []string{}
						for _, entity := range entities {
							m.previewColumn.items = append(m.previewColumn.items, formatEntityForDisplay(entity))
						}
						m.previewColumn.entities = entities
					}
				case "json":
					if entityData, ok := msg.data.(map[string]interface{}); ok {
						m.previewColumn.title = "JSON Preview"
						jsonData, err := json.MarshalIndent(entityData, "", "  ")
						if err != nil {
							m.previewColumn.items = []string{fmt.Sprintf("Error formatting JSON: %v", err)}
						} else {
							m.previewColumn.items = strings.Split(string(jsonData), "\n")
						}
					}
				case "function":
					if funcData, ok := msg.data.(map[string]interface{}); ok {
						m.previewColumn.title = "Function Preview"
						m.previewColumn.items = []string{
							fmt.Sprintf("Name: %v", funcData["name"]),
							fmt.Sprintf("Type: %v", funcData["type"]),
							"",
							fmt.Sprintf("%v", funcData["note"]),
						}
					}
				case "navigation":
					if navData, ok := msg.data.(map[string]interface{}); ok {
						m.previewColumn.title = "Navigation"
						m.previewColumn.items = []string{
							fmt.Sprintf("URI: %v", navData["uri"]),
							"",
							fmt.Sprintf("%v", navData["note"]),
						}
					}
				case "none":
					m.previewColumn.title = "Preview"
					m.previewColumn.items = []string{"No preview available at this level"}
				}
			}
		}

	case entityDetailMsg:
		m.loading = false
		m.logs = append(m.logs, fmt.Sprintf("Read detailed entity %s from %s", msg.entityKey, msg.entitySet))
		
		// Update the details column with the detailed entity
		for i := range m.columns {
			if m.columns[i].title == "Details" && m.columns[i].isDetails {
				// Replace the stored entity with the detailed one
				m.columns[i].entities = []map[string]interface{}{msg.entity}
				
				// Update JSON display
				jsonData, err := json.MarshalIndent(msg.entity, "", "  ")
				if err != nil {
					m.columns[i].items = []string{fmt.Sprintf("Error formatting JSON: %v", err)}
				} else {
					m.columns[i].items = strings.Split(string(jsonData), "\n")
				}
				
				// Reset cursor and scroll
				m.columns[i].cursor = 0
				m.columns[i].scrollOffset = 0
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
		// Handle modal editor first
		if m.modalEditor {
			switch msg.String() {
			case "ctrl+c", "q", "f10":
				return m, tea.Quit
			case "esc":
				// Cancel modal editor
				m.modalEditor = false
				m.modalContent = nil
				m.modalCursor = 0
				m.modalScroll = 0
				m.logs = append(m.logs, "Modal editor cancelled")
				return m, nil
			case "f2":
				// Save changes and close modal
				return m.saveModalChanges(), nil
			case "up", "k":
				if m.modalCursor > 0 {
					m.modalCursor--
					if m.modalCursor < m.modalScroll {
						m.modalScroll = m.modalCursor
					}
				}
			case "down", "j":
				if m.modalCursor < len(m.modalContent)-1 {
					m.modalCursor++
					modalHeight := int(float64(m.height) * 0.95) - 4
					if m.modalCursor >= m.modalScroll+modalHeight {
						m.modalScroll = m.modalCursor - modalHeight + 1
					}
				}
			case "pgup":
				modalHeight := int(float64(m.height) * 0.95) - 4
				newCursor := m.modalCursor - modalHeight
				if newCursor < 0 {
					newCursor = 0
				}
				m.modalCursor = newCursor
				m.modalScroll = newCursor
			case "pgdown":
				modalHeight := int(float64(m.height) * 0.95) - 4
				newCursor := m.modalCursor + modalHeight
				if newCursor >= len(m.modalContent) {
					newCursor = len(m.modalContent) - 1
				}
				m.modalCursor = newCursor
				if m.modalCursor >= m.modalScroll+modalHeight {
					m.modalScroll = m.modalCursor - modalHeight + 1
				}
			case "home":
				m.modalCursor = 0
				m.modalScroll = 0
			case "end":
				if len(m.modalContent) > 0 {
					m.modalCursor = len(m.modalContent) - 1
					modalHeight := int(float64(m.height) * 0.95) - 4
					if len(m.modalContent) > modalHeight {
						m.modalScroll = len(m.modalContent) - modalHeight
					} else {
						m.modalScroll = 0
					}
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q", "f10":
			return m, tea.Quit

		case "up", "k":
			if m.editMode {
				// In edit mode, move cursor up in text
				if m.editCursor > 0 {
					m.editCursor--
				}
			} else if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				if col.cursor > 0 {
					col.cursor--
					// Ensure cursor is visible in viewport for all columns
					if col.cursor < col.scrollOffset {
						col.scrollOffset = col.cursor
					}
					// Update preview when cursor moves (except in details view)
					if !col.isDetails {
						return m, m.updatePreview()
					}
				}
			}

		case "down", "j":
			if m.editMode {
				// In edit mode, move cursor down in text
				if m.editCursor < len(m.editContent)-1 {
					m.editCursor++
				}
			} else if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				if col.cursor < len(col.items)-1 {
					col.cursor++
					// Ensure cursor is visible in viewport for all columns
					visibleHeight := col.height - 2 // Account for borders
					if col.cursor >= col.scrollOffset+visibleHeight {
						col.scrollOffset = col.cursor - visibleHeight + 1
					}
					// Update preview when cursor moves (except in details view)
					if !col.isDetails {
						return m, m.updatePreview()
					}
				}
			}

		case "right", "l", "enter":
			if !m.editMode {
				return m.drillDown()
			}

		case "left", "h", "esc":
			if m.editMode {
				// Cancel edit mode
				m.editMode = false
				m.logs = append(m.logs, "Edit cancelled")
				return m, nil
			}
			newModel := m.goBack()
			return newModel, newModel.updatePreview()

		case "f2":
			// Save in modal editor (if in modal), otherwise create entity
			if m.modalEditor {
				return m.saveModalChanges(), nil
			}
			// TODO: Create entity
		case "f3":
			return m.readEntityDetails()
		case "f4":
			// TODO: Update entity
		case "f5":
			// Open modal editor for entity details
			return m.openModalEditor(), nil
		case "f7":
			// TODO: Filter
		case "f8":
			// TODO: Delete entity
		case "f9":
			m.showLogs = !m.showLogs
			
		case "pgup":
			if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				pageSize := col.height - 2 // Account for borders
				newCursor := col.cursor - pageSize
				if newCursor < 0 {
					newCursor = 0
				}
				col.cursor = newCursor
				col.scrollOffset = newCursor
			}
			
		case "pgdown":
			if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				pageSize := col.height - 2
				newCursor := col.cursor + pageSize
				if newCursor >= len(col.items) {
					newCursor = len(col.items) - 1
				}
				col.cursor = newCursor
				visibleHeight := col.height - 2
				if col.cursor >= col.scrollOffset+visibleHeight {
					col.scrollOffset = col.cursor - visibleHeight + 1
				}
			}
			
		case "home":
			if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				col.cursor = 0
				col.scrollOffset = 0
			}
			
		case "end":
			if m.activeColumn < len(m.columns) {
				col := &m.columns[m.activeColumn]
				if len(col.items) > 0 {
					col.cursor = len(col.items) - 1
					visibleHeight := col.height - 2
					if len(col.items) > visibleHeight {
						col.scrollOffset = len(col.items) - visibleHeight
					} else {
						col.scrollOffset = 0
					}
				}
			}
		}
	}

	return m, nil
}

func (m *model) updateColumnSizes() {
	if len(m.columns) == 0 {
		return
	}

	// Reserve space for preview column (30% of total width)
	previewWidth := int(float64(m.width) * 0.3)
	if m.previewColumn != nil {
		m.previewColumn.width = previewWidth
		m.previewColumn.height = m.height - 4
	}

	totalWidth := m.width - previewWidth
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
		for i, svc := range m.services {
			if svc.Name == selectedItem {
				m.serviceIndex = i
				m.odata = NewODataServiceWithAuth(svc.URL, svc.Username, svc.Password)
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
		cmd = tea.Batch(loadEntitySets(m.odata), m.updatePreview())
		
	case 1: // EntitySets -> Entities
		// Extract entity set name from display text (remove capabilities part)
		entitySetName := strings.Split(selectedItem, " [")[0]
		newColumn = column{
			title:   entitySetName,
			items:   []string{"Loading..."},
			cursor:  0,
			focused: false,
		}
		m.columns = append(m.columns, newColumn)
		m.activeColumn++
		m.columns[m.activeColumn].focused = true
		m.updateColumnSizes()
		m.loading = true
		cmd = tea.Batch(loadEntities(m.odata, entitySetName), m.updatePreview())
		
	case 2: // Entities -> JSON Details
		// Get the actual entity data from the previous column
		prevCol := m.columns[m.activeColumn]
		if prevCol.cursor < len(prevCol.entities) {
			selectedEntity := prevCol.entities[prevCol.cursor]
			
			// Format entity as JSON
			jsonData, err := json.MarshalIndent(selectedEntity, "", "  ")
			if err != nil {
				newColumn = column{
					title:     "Details",
					items:     []string{fmt.Sprintf("Error formatting entity: %v", err)},
					cursor:    0,
					focused:   false,
					isDetails: true,
				}
			} else {
				// Split JSON into lines for display
				lines := strings.Split(string(jsonData), "\n")
				newColumn = column{
					title:     "Details",
					items:     lines,
					cursor:    0,
					focused:   false,
					isDetails: true,
					entities:  []map[string]interface{}{selectedEntity}, // Store the entity for editing
				}
			}
		} else {
			newColumn = column{
				title:     "Details",
				items:     []string{"No entity data available"},
				cursor:    0,
				focused:   false,
				isDetails: true,
			}
		}
		m.columns = append(m.columns, newColumn)
		m.activeColumn++
		m.columns[m.activeColumn].focused = true
		m.updateColumnSizes()
		
	default:
		// We're already at JSON level (column 3), don't create more columns
		// TODO: Handle navigation properties here
		return m, nil
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

// readEntityDetails reads the full details of the currently selected entity
func (m model) readEntityDetails() (tea.Model, tea.Cmd) {
	// Only works when we're viewing entities (not in details view)
	if m.activeColumn < 0 || m.activeColumn >= len(m.columns) {
		return m, nil
	}
	
	currentCol := m.columns[m.activeColumn]
	if currentCol.isDetails || len(currentCol.entities) == 0 || currentCol.cursor >= len(currentCol.entities) {
		m.logs = append(m.logs, "F3: Select an entity in the entity list to read details")
		return m, nil
	}
	
	// Get the selected entity
	selectedEntity := currentCol.entities[currentCol.cursor]
	entitySetName := currentCol.title
	
	// Extract the key value(s) from the entity
	entityKey := extractEntityKey(selectedEntity)
	if entityKey == "" {
		m.logs = append(m.logs, "F3: Could not determine entity key for detailed read")
		return m, nil
	}
	
	m.loading = true
	m.logs = append(m.logs, fmt.Sprintf("Reading detailed entity %s from %s...", entityKey, entitySetName))
	
	return m, func() tea.Msg {
		entity, err := m.odata.GetEntity(entitySetName, entityKey)
		if err != nil {
			return errorMsg{err: err.Error(), context: fmt.Sprintf("readEntity(%s, %s)", entitySetName, entityKey)}
		}
		return entityDetailMsg{entitySet: entitySetName, entityKey: entityKey, entity: entity}
	}
}

// extractEntityKey extracts the primary key value from an entity
func extractEntityKey(entity map[string]interface{}) string {
	// First, check for __metadata.id or __metadata.uri which contains the proper key
	if metadata, ok := entity["__metadata"].(map[string]interface{}); ok {
		if id, ok := metadata["id"].(string); ok {
			// Extract key from URI like "https://host/service/EntitySet('key')"
			if lastParen := strings.LastIndex(id, "("); lastParen != -1 {
				if endParen := strings.Index(id[lastParen:], ")"); endParen != -1 {
					return id[lastParen+1 : lastParen+endParen]
				}
			}
		}
		if uri, ok := metadata["uri"].(string); ok {
			// Extract key from URI like "https://host/service/EntitySet('key')"
			if lastParen := strings.LastIndex(uri, "("); lastParen != -1 {
				if endParen := strings.Index(uri[lastParen:], ")"); endParen != -1 {
					return uri[lastParen+1 : lastParen+endParen]
				}
			}
		}
	}
	
	// Fallback: Common key field patterns
	keyFields := []string{"Program", "Class", "Interface", "Package", "Function", 
		"ID", "Id", "Key", "Code", "Number", 
		"ProductID", "CategoryID", "CustomerID", "OrderID", "EmployeeID"}
	
	// Check for key fields
	for _, field := range keyFields {
		if val := entity[field]; val != nil {
			// Format the key value for OData URL
			if str, ok := val.(string); ok {
				// String keys need to be quoted
				return fmt.Sprintf("'%s'", str)
			} else {
				// Numeric keys don't need quotes
				return fmt.Sprintf("%v", val)
			}
		}
	}
	
	// Last fallback: look for any field that might be a key
	for k, v := range entity {
		if v != nil && !strings.HasPrefix(k, "__") && !strings.Contains(strings.ToLower(k), "date") {
			if str, ok := v.(string); ok && str != "" {
				return fmt.Sprintf("'%s'", str)
			} else if num := v; num != nil {
				return fmt.Sprintf("%v", num)
			}
		}
	}
	
	return ""
}

// updatePreview generates a preview based on current cursor position
func (m model) updatePreview() tea.Cmd {
	if m.activeColumn >= len(m.columns) {
		return nil
	}

	currentCol := m.columns[m.activeColumn]
	if currentCol.cursor >= len(currentCol.items) {
		return nil
	}

	selectedItem := currentCol.items[currentCol.cursor]
	m.previewLoading = true

	switch m.activeColumn {
	case 0: // Service selection - preview entity sets
		return func() tea.Msg {
			for _, svc := range m.services {
				if svc.Name == selectedItem {
					odataService := NewODataServiceWithAuth(svc.URL, svc.Username, svc.Password)
					entitySets, err := odataService.GetEntitySets()
					if err != nil {
						return previewMsg{errorMsg: err.Error()}
					}
					return previewMsg{previewType: "entitysets", data: entitySets}
				}
			}
			return previewMsg{errorMsg: "Service not found"}
		}

	case 1: // EntitySets - preview entities
		if m.odata != nil {
			entitySetName := strings.Split(selectedItem, " [")[0]
			
			// Check if this is a function import
			if strings.HasPrefix(entitySetName, "[FUNC] ") {
				funcName := strings.TrimPrefix(entitySetName, "[FUNC] ")
				return func() tea.Msg {
					return previewMsg{previewType: "function", data: map[string]interface{}{
						"name": funcName,
						"note": "Function Import - press Enter to view parameters and execute",
						"type": "Function Import"}}
				}
			}
			
			return func() tea.Msg {
				entities, _, err := m.odata.GetEntitiesWithCount(entitySetName, 10) // Default to 10 for preview
				if err != nil {
					return previewMsg{errorMsg: err.Error()}
				}
				return previewMsg{previewType: "entities", data: entities}
			}
		}

	default: // Entity list or JSON details
		if currentCol.isDetails {
			// We're in JSON view - only preview if cursor is on a navigation association
			if currentCol.cursor < len(currentCol.items) {
				currentLine := currentCol.items[currentCol.cursor]
				// Check if this line contains a deferred navigation property
				if strings.Contains(currentLine, "__deferred") && strings.Contains(currentLine, "uri") {
					// Extract URI from the line
					if uriStart := strings.Index(currentLine, "https://"); uriStart != -1 {
						uriEnd := strings.Index(currentLine[uriStart:], `"`)
						if uriEnd != -1 {
							uri := currentLine[uriStart : uriStart+uriEnd]
							return func() tea.Msg {
								// For now, show the URI as preview
								// TODO: Actually fetch the related entity
								return previewMsg{previewType: "navigation", data: map[string]interface{}{"uri": uri, "note": "Navigation property - press Enter to follow"}}
							}
						}
					}
				}
			}
			// No preview for regular JSON lines
			return func() tea.Msg {
				return previewMsg{previewType: "none", data: nil}
			}
		} else if currentCol.entities != nil && currentCol.cursor < len(currentCol.entities) {
			// Entity list - preview JSON
			selectedEntity := currentCol.entities[currentCol.cursor]
			return func() tea.Msg {
				return previewMsg{previewType: "json", data: selectedEntity}
			}
		}
	}

	return nil
}

func (m model) toggleEditMode() model {
	// Only allow edit mode when viewing details of an entity
	if m.activeColumn >= 0 && m.activeColumn < len(m.columns) {
		currentCol := m.columns[m.activeColumn]
		if currentCol.isDetails && len(currentCol.entities) > 0 {
			m.editMode = !m.editMode
			if m.editMode {
				// Copy current JSON content for editing
				m.editContent = make([]string, len(currentCol.items))
				copy(m.editContent, currentCol.items)
				m.editCursor = currentCol.cursor
				m.logs = append(m.logs, "Entered EDIT mode - F5 to save, ESC to cancel")
			} else {
				m.logs = append(m.logs, "Exited EDIT mode")
			}
		} else {
			m.logs = append(m.logs, "Edit mode only available for entity details")
		}
	}
	return m
}

func (m model) saveChanges() model {
	if !m.editMode || m.activeColumn >= len(m.columns) {
		return m
	}
	
	currentCol := &m.columns[m.activeColumn]
	if !currentCol.isDetails || len(currentCol.entities) == 0 {
		m.logs = append(m.logs, "No entity data to save")
		return m
	}

	// Try to parse the edited JSON
	jsonContent := strings.Join(m.editContent, "\n")
	var updatedEntity map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &updatedEntity); err != nil {
		m.logs = append(m.logs, fmt.Sprintf("Invalid JSON: %v", err))
		return m
	}

	// Update the stored entity
	currentCol.entities[0] = updatedEntity
	
	// Update the display
	jsonData, err := json.MarshalIndent(updatedEntity, "", "  ")
	if err != nil {
		m.logs = append(m.logs, fmt.Sprintf("Error formatting JSON: %v", err))
		return m
	}
	
	currentCol.items = strings.Split(string(jsonData), "\n")
	m.editMode = false
	m.logs = append(m.logs, "Changes saved locally (not persisted to server)")
	
	return m
}

// openModalEditor opens a full-screen modal editor for entity details
func (m model) openModalEditor() model {
	// Only allow modal editor when viewing details of an entity
	if m.activeColumn >= 0 && m.activeColumn < len(m.columns) {
		currentCol := m.columns[m.activeColumn]
		if currentCol.isDetails && len(currentCol.entities) > 0 {
			m.modalEditor = true
			// Copy current JSON content for editing
			m.modalContent = make([]string, len(currentCol.items))
			copy(m.modalContent, currentCol.items)
			m.modalCursor = currentCol.cursor
			m.modalScroll = currentCol.scrollOffset
			m.logs = append(m.logs, "Modal editor opened - F2 to save, ESC to cancel")
		} else {
			m.logs = append(m.logs, "Modal editor only available for entity details")
		}
	}
	return m
}

// saveModalChanges saves changes from modal editor and closes it
func (m model) saveModalChanges() model {
	if !m.modalEditor || m.activeColumn >= len(m.columns) {
		return m
	}
	
	currentCol := &m.columns[m.activeColumn]
	if !currentCol.isDetails || len(currentCol.entities) == 0 {
		m.logs = append(m.logs, "No entity data to save")
		m.modalEditor = false
		return m
	}

	// Try to parse the edited JSON
	jsonContent := strings.Join(m.modalContent, "\n")
	var updatedEntity map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &updatedEntity); err != nil {
		m.logs = append(m.logs, fmt.Sprintf("Invalid JSON: %v", err))
		return m
	}

	// Update the stored entity
	currentCol.entities[0] = updatedEntity
	
	// Update the display
	jsonData, err := json.MarshalIndent(updatedEntity, "", "  ")
	if err != nil {
		m.logs = append(m.logs, fmt.Sprintf("Error formatting JSON: %v", err))
		m.modalEditor = false
		return m
	}
	
	currentCol.items = strings.Split(string(jsonData), "\n")
	currentCol.cursor = 0
	currentCol.scrollOffset = 0
	
	// Close modal
	m.modalEditor = false
	m.modalContent = nil
	m.modalCursor = 0
	m.modalScroll = 0
	
	m.logs = append(m.logs, "Changes saved locally (not persisted to server)")
	
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
	if m.previewColumn != nil {
		m.previewColumn.height = bodyHeight
	}

	var columns []string
	
	for i, col := range m.columns {
		columns = append(columns, m.renderColumn(col, i == m.activeColumn))
	}
	
	// Add preview column
	if m.previewColumn != nil {
		previewTitle := m.previewColumn.title
		if m.previewLoading {
			previewTitle += " (Loading...)"
		}
		previewCol := *m.previewColumn
		previewCol.title = previewTitle
		columns = append(columns, m.renderColumn(previewCol, false))
	}

	headerText := "OData Navigator"
	if m.serviceIndex >= 0 && m.serviceIndex < len(m.services) {
		headerText = fmt.Sprintf("OData Navigator - %s", m.services[m.serviceIndex].Name)
	}
	headerText += " - Use arrows to navigate, Enter to drill down, rightmost column shows preview"
	
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render(headerText)

	footerText := "F2:Create F3:Read F4:Update F5:ModalEdit F7:Filter F8:Delete F9:Toggle Logs F10:Exit | ESC:Back"
	if m.modalEditor {
		footerText = "MODAL EDITOR - F2:Save ESC:Cancel | Navigation: Up/Down/PgUp/PgDown/Home/End"
	} else if m.editMode {
		footerText = "EDIT MODE - F5:Save ESC:Cancel | " + footerText
	}
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(footerText)

	body := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	
	// Build the complete view
	parts := []string{header, "", body}
	
	if m.showLogs {
		logView := m.renderLogs(logHeight)
		parts = append(parts, logView)
	}
	
	parts = append(parts, "", footer)
	
	view := lipgloss.JoinVertical(lipgloss.Left, parts...)
	
	// Overlay modal editor if active
	if m.modalEditor {
		view = m.renderModalOverlay(view)
	}
	
	return view
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

// renderModalOverlay renders a modal editor overlay on top of the main view
func (m model) renderModalOverlay(baseView string) string {
	// Calculate modal dimensions (95% of screen)
	modalWidth := int(float64(m.width) * 0.95)
	modalHeight := int(float64(m.height) * 0.95)
	
	// Calculate content dimensions
	contentHeight := modalHeight - 4 // Account for borders and header
	
	// Prepare modal content
	var visibleContent []string
	if len(m.modalContent) > 0 {
		endIdx := m.modalScroll + contentHeight
		if endIdx > len(m.modalContent) {
			endIdx = len(m.modalContent)
		}
		visibleContent = m.modalContent[m.modalScroll:endIdx]
	}
	
	// Add cursor indicator and line numbers
	var renderedLines []string
	for i, line := range visibleContent {
		lineNum := m.modalScroll + i
		prefix := fmt.Sprintf("%4d ", lineNum+1)
		
		if lineNum == m.modalCursor {
			// Highlight current line
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("99")).
				Foreground(lipgloss.Color("0")).
				Render(prefix + line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render(prefix) + line
		}
		renderedLines = append(renderedLines, line)
	}
	
	// Fill remaining space with empty lines
	for len(renderedLines) < contentHeight {
		renderedLines = append(renderedLines, "")
	}
	
	content := strings.Join(renderedLines, "\n")
	
	// Create modal box
	modalStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Height(modalHeight).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("99")).
		Background(lipgloss.Color("0")).
		Foreground(lipgloss.Color("15"))
	
	title := " Modal Editor - F2: Save | ESC: Cancel "
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color("99")).
		Foreground(lipgloss.Color("0")).
		Padding(0, 1)
	
	// Render modal with title
	modal := titleStyle.Render(title) + "\n" + content
	
	// Calculate position to center modal
	x := (m.width - modalWidth) / 2
	y := (m.height - modalHeight) / 2
	
	// Create overlay by splitting base view into lines and inserting modal
	baseLines := strings.Split(baseView, "\n")
	
	// Ensure we have enough lines
	for len(baseLines) < m.height {
		baseLines = append(baseLines, "")
	}
	
	modalLines := strings.Split(modalStyle.Render(modal), "\n")
	
	// Overlay modal lines onto base view
	for i, modalLine := range modalLines {
		if y+i >= 0 && y+i < len(baseLines) {
			if x >= 0 && x+len(modalLine) <= len(baseLines[y+i]) {
				// Simple overlay - just replace the section
				line := baseLines[y+i]
				if x+len(modalLine) < len(line) {
					baseLines[y+i] = line[:x] + modalLine + line[x+len(modalLine):]
				} else {
					baseLines[y+i] = line[:x] + modalLine
				}
			} else {
				// Modal extends beyond base line, just replace the line
				baseLines[y+i] = strings.Repeat(" ", x) + modalLine
			}
		}
	}
	
	return strings.Join(baseLines, "\n")
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

	// If in edit mode and this is the active column with details
	if m.editMode && isActive && col.isDetails {
		// Show editable content with EDIT indicator in title
		titleStyle = titleStyle.Background(lipgloss.Color("208")).Foreground(lipgloss.Color("0"))
		
		for i, item := range m.editContent {
			style := lipgloss.NewStyle().Padding(0, 1)
			
			if i == m.editCursor {
				// Highlight current edit line with different color
				style = style.Background(lipgloss.Color("208")).Foreground(lipgloss.Color("0"))
				item = "► " + item // Add edit cursor indicator
			} else {
				// Make non-cursor lines stand out as editable
				style = style.Background(lipgloss.Color("235")).Foreground(lipgloss.Color("15"))
			}
			
			items = append(items, style.Render(item))
		}
	} else {
		// Normal display mode
		// Calculate viewport for scrolling on all columns
		startIdx := 0
		endIdx := len(col.items)
		
		if col.height > 2 {
			// Implement viewport scrolling for all columns
			visibleHeight := col.height - 2 // Account for borders
			startIdx = col.scrollOffset
			endIdx = startIdx + visibleHeight
			if endIdx > len(col.items) {
				endIdx = len(col.items)
			}
		}
		
		for i := startIdx; i < endIdx; i++ {
			if i >= len(col.items) {
				break
			}
			item := col.items[i]
			style := lipgloss.NewStyle().Padding(0, 1)
			
			// Color function imports and more indicators differently
			if strings.HasPrefix(item, "[FUNC]") {
				if i == col.cursor && isActive {
					style = style.Background(lipgloss.Color("99")).Foreground(lipgloss.Color("0"))
				} else if i == col.cursor {
					style = style.Background(lipgloss.Color("241")).Foreground(lipgloss.Color("15"))
				} else {
					// Function imports in purple/magenta
					style = style.Foreground(lipgloss.Color("13"))
				}
			} else if strings.HasPrefix(item, "[...more") {
				// More indicator in gray/dimmed
				if i == col.cursor && isActive {
					style = style.Background(lipgloss.Color("99")).Foreground(lipgloss.Color("0"))
				} else if i == col.cursor {
					style = style.Background(lipgloss.Color("241")).Foreground(lipgloss.Color("15"))
				} else {
					style = style.Foreground(lipgloss.Color("8")) // Gray/dimmed
				}
			} else {
				if i == col.cursor && isActive {
					style = style.Background(lipgloss.Color("99")).Foreground(lipgloss.Color("0"))
				} else if i == col.cursor {
					style = style.Background(lipgloss.Color("241")).Foreground(lipgloss.Color("15"))
				}
				
				// Handle grayed out additional info
				if strings.Contains(item, " | ") {
					parts := strings.SplitN(item, " | ", 2)
					if len(parts) == 2 {
						// Style: key (normal) + " | " + description (grayed)
						mainPart := parts[0]
						grayPart := " | " + parts[1]
						
						if i == col.cursor && isActive {
							item = mainPart + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(grayPart)
						} else if i == col.cursor {
							item = mainPart + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(grayPart)
						} else {
							item = mainPart + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(grayPart)
						}
					}
				}
			}
			
			items = append(items, style.Render(item))
		}
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

	// Modify title for edit mode and add scroll indicator
	title := col.title
	if m.editMode && isActive && col.isDetails {
		title = "[EDIT] " + col.title
	}
	// Add scroll indicator for any column with large content
	if len(col.items) > col.height-2 && col.height > 2 {
		totalLines := len(col.items)
		visibleHeight := col.height - 2
		currentPos := col.scrollOffset + 1
		endPos := currentPos + visibleHeight - 1
		if endPos > totalLines {
			endPos = totalLines
		}
		title = fmt.Sprintf("%s (%d-%d/%d)", col.title, currentPos, endPos, totalLines)
	}
	
	return columnStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(title),
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