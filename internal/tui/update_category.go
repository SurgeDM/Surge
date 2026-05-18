package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/utils"
)

func (m *RootModel) catMgrBeginAdd() {
	newCat := config.Category{Name: "New Category"}
	m.Settings.Categories.Categories = append(m.Settings.Categories.Categories, newCat)
	m.catMgrCursor = len(m.Settings.Categories.Categories) - 1
	m.catMgrIsNew = true
	m.catMgrEditing = true
	m.catMgrError = ""
	m.catMgrEditField = 0
	m.catMgrInputs[0].SetValue(newCat.Name)
	m.catMgrInputs[1].SetValue(newCat.Description)
	m.catMgrInputs[2].SetValue(newCat.Pattern)
	m.catMgrInputs[3].SetValue(newCat.Path)
	m.updateCategoryInputWidthsForViewport()
	m.catMgrInputs[0].Focus()
}

func (m *RootModel) blurAllCatInputs() {
	for i := range m.catMgrInputs {
		m.catMgrInputs[i].Blur()
	}
}

func (m *RootModel) normalizeCategoryManagerSelection() {
	if m.Settings == nil {
		return
	}

	cats := m.Settings.Categories.Categories
	maxCursor := len(cats)
	if m.catMgrEditing {
		if len(cats) == 0 {
			m.catMgrCursor = 0
			m.catMgrEditField = 0
			m.catMgrEditing = false
			m.catMgrIsNew = false
			m.blurAllCatInputs()
			return
		}
		maxCursor = len(cats) - 1
	}

	if m.catMgrCursor < 0 {
		m.catMgrCursor = 0
	}
	if m.catMgrCursor > maxCursor {
		m.catMgrCursor = maxCursor
	}

	if m.catMgrEditField < 0 {
		m.catMgrEditField = 0
	}
	if m.catMgrEditField > 3 {
		m.catMgrEditField = 3
	}
}

func (m *RootModel) updateCategoryInputWidthsForViewport() {
	modalWidth, _ := GetSettingsDimensions(m.width, m.height)

	var targetWidth int
	if modalWidth >= 76 {
		_, rightWidth := CalculateTwoColumnWidths(modalWidth, 28, 24)
		targetWidth = rightWidth - 10
	} else {
		targetWidth = modalWidth - 14
	}

	if targetWidth < 10 {
		targetWidth = 10
	}
	if targetWidth > 64 {
		targetWidth = 64
	}

	for i := range m.catMgrInputs {
		m.catMgrInputs[i].SetWidth(targetWidth)
	}
}

func (m RootModel) updateCategoryManager(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.normalizeCategoryManagerSelection()
	cats := m.Settings.Categories.Categories

	// Handle editing mode
	if m.catMgrEditing {
		if key.Matches(msg, m.keys.CategoryMgr.Close) {
			wasNew := m.catMgrIsNew
			// Cancel editing
			m.catMgrEditing = false
			m.blurAllCatInputs()

			// If was adding new, remove the placeholder
			if wasNew && m.catMgrCursor < len(m.Settings.Categories.Categories) {
				m.Settings.Categories.Categories = append(
					m.Settings.Categories.Categories[:m.catMgrCursor],
					m.Settings.Categories.Categories[m.catMgrCursor+1:]...,
				)
				if m.catMgrCursor > 0 {
					m.catMgrCursor--
				}
			}
			m.catMgrIsNew = false
			m.catMgrError = ""
			return m, nil
		}
		if key.Matches(msg, m.keys.CategoryMgr.Tab) {
			m.catMgrError = ""
			// On Path field, open file picker for directory browsing
			if m.catMgrEditField == 3 {
				originalPath := m.catMgrInputs[3].Value()
				browseDir := strings.TrimSpace(originalPath)
				if browseDir == "" {
					browseDir = m.Settings.General.DefaultDownloadDir
				}
				if browseDir == "" {
					browseDir = m.PWD
				}
				return m, m.openDirectoryPicker(FilePickerOriginCategory, originalPath, browseDir, false, true)
			}
			// Cycle fields
			m.catMgrInputs[m.catMgrEditField].Blur()
			m.catMgrEditField = (m.catMgrEditField + 1) % 4
			m.catMgrInputs[m.catMgrEditField].Focus()
			return m, nil
		}
		if key.Matches(msg, m.keys.CategoryMgr.Up) {
			m.catMgrError = ""
			m.catMgrInputs[m.catMgrEditField].Blur()
			m.catMgrEditField--
			if m.catMgrEditField < 0 {
				m.catMgrEditField = 3
			}
			m.catMgrInputs[m.catMgrEditField].Focus()
			return m, nil
		}
		if key.Matches(msg, m.keys.CategoryMgr.Down) {
			m.catMgrError = ""
			m.catMgrInputs[m.catMgrEditField].Blur()
			m.catMgrEditField = (m.catMgrEditField + 1) % 4
			m.catMgrInputs[m.catMgrEditField].Focus()
			return m, nil
		}
		if key.Matches(msg, m.keys.CategoryMgr.Edit) {
			// Save edits
			if m.catMgrCursor < 0 || m.catMgrCursor >= len(m.Settings.Categories.Categories) {
				m.catMgrError = "Invalid category selection"
				utils.Debug("Category Manager Error: %s", m.catMgrError)
				return m, nil
			}

			name := strings.TrimSpace(m.catMgrInputs[0].Value())
			description := strings.TrimSpace(m.catMgrInputs[1].Value())
			pattern := strings.TrimSpace(m.catMgrInputs[2].Value())
			path := strings.TrimSpace(m.catMgrInputs[3].Value())

			if name == "" {
				m.catMgrError = "Category name cannot be empty"
				utils.Debug("Category Manager Error: %s", m.catMgrError)
				return m, nil
			}
			if pattern == "" {
				m.catMgrError = "Category pattern cannot be empty"
				utils.Debug("Category Manager Error: %s", m.catMgrError)
				return m, nil
			}
			if _, err := regexp.Compile(pattern); err != nil {
				m.catMgrError = fmt.Sprintf("Invalid regex pattern: %v", err)
				utils.Debug("Category Manager Error: %s", m.catMgrError)
				return m, nil
			}
			if path == "" {
				m.catMgrError = "Category path cannot be empty"
				utils.Debug("Category Manager Error: %s", m.catMgrError)
				return m, nil
			}

			target := &m.Settings.Categories.Categories[m.catMgrCursor]
			target.Name = name
			target.Description = description
			target.Pattern = pattern
			target.Path = filepath.Clean(path)

			if m.catMgrIsNew {
				utils.Debug("Category Added: %s (Path: %s)", name, path)
			} else {
				utils.Debug("Category Updated: %s (Path: %s)", name, path)
			}

			m.catMgrEditing = false
			m.catMgrIsNew = false
			m.catMgrError = ""

			m.blurAllCatInputs()

			return m, nil
		}

		// Pass to active text input
		var cmd tea.Cmd
		m.catMgrInputs[m.catMgrEditField], cmd = m.catMgrInputs[m.catMgrEditField].Update(msg)
		return m, cmd
	}

	// Not editing - handle navigation
	if key.Matches(msg, m.keys.CategoryMgr.Close) {
		_ = m.persistSettings()
		m.state = SettingsState
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Up) {
		m.catMgrError = ""
		if m.catMgrCursor > 0 {
			m.catMgrCursor--
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.CategoryMgr.Down) {
		m.catMgrError = ""
		if m.catMgrCursor < len(cats) { // len(cats) = "+Add" row
			m.catMgrCursor++
		}
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Toggle) {
		m.Settings.Categories.CategoryEnabled = !m.Settings.Categories.CategoryEnabled
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Delete) {
		m.catMgrError = ""
		if m.catMgrCursor < len(cats) {
			deletedName := cats[m.catMgrCursor].Name
			m.Settings.Categories.Categories = append(
				m.Settings.Categories.Categories[:m.catMgrCursor],
				m.Settings.Categories.Categories[m.catMgrCursor+1:]...,
			)
			utils.Debug("Category Removed: %s", deletedName)
			if m.catMgrCursor >= len(m.Settings.Categories.Categories) && m.catMgrCursor > 0 {
				m.catMgrCursor--
			}
		}
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Add) {
		m.catMgrBeginAdd()
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Edit) {
		if m.catMgrCursor < len(cats) {
			// Edit existing
			cat := cats[m.catMgrCursor]
			m.catMgrEditing = true
			m.catMgrEditField = 0
			m.catMgrInputs[0].SetValue(cat.Name)
			m.catMgrInputs[1].SetValue(cat.Description)
			m.catMgrInputs[2].SetValue(cat.Pattern)
			m.catMgrInputs[3].SetValue(cat.Path)
			m.updateCategoryInputWidthsForViewport()
			m.catMgrInputs[0].Focus()
		} else {
			m.catMgrBeginAdd()
		}
		return m, nil
	}

	return m, nil
}
