package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/surge-downloader/surge/internal/config"
)

func (m *RootModel) catMgrBeginAdd() {
	newCat := config.Category{Name: "New Category"}
	m.Settings.General.Categories = append(m.Settings.General.Categories, newCat)
	m.catMgrCursor = len(m.Settings.General.Categories) - 1
	m.catMgrIsNew = true
	m.catMgrEditing = true
	m.catMgrEditField = 0
	m.catMgrInputs[0].SetValue(newCat.Name)
	m.catMgrInputs[1].SetValue(newCat.Description)
	m.catMgrInputs[2].SetValue(newCat.Pattern)
	m.catMgrInputs[3].SetValue(newCat.Path)
	m.catMgrInputs[0].Focus()
}

func (m *RootModel) blurAllCatInputs() {
	for i := range m.catMgrInputs {
		m.catMgrInputs[i].Blur()
	}
}

func (m RootModel) updateCategoryManager(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cats := m.Settings.General.Categories

	// Handle editing mode
	if m.catMgrEditing {
		if key.Matches(msg, m.keys.CategoryMgr.Close) {
			wasNew := m.catMgrIsNew
			// Cancel editing
			m.catMgrEditing = false
			m.blurAllCatInputs()

			// If was adding new, remove the placeholder
			if wasNew && m.catMgrCursor < len(m.Settings.General.Categories) {
				m.Settings.General.Categories = append(
					m.Settings.General.Categories[:m.catMgrCursor],
					m.Settings.General.Categories[m.catMgrCursor+1:]...,
				)
				if m.catMgrCursor > 0 {
					m.catMgrCursor--
				}
			}
			m.catMgrIsNew = false
			return m, nil
		}
		if key.Matches(msg, m.keys.CategoryMgr.Tab) {
			// On Path field, open file picker for directory browsing
			if m.catMgrEditField == 3 {
				browseDir := strings.TrimSpace(m.catMgrInputs[3].Value())
				if browseDir == "" {
					browseDir = m.Settings.General.DefaultDownloadDir
				}
				if browseDir == "" {
					browseDir = m.PWD
				}
				m.catMgrFileBrowsing = true
				m.state = FilePickerState
				m.filepicker = newFilepicker(browseDir)
				return m, m.filepicker.Init()
			}
			// Cycle fields
			m.catMgrInputs[m.catMgrEditField].Blur()
			m.catMgrEditField = (m.catMgrEditField + 1) % 4
			m.catMgrInputs[m.catMgrEditField].Focus()
			return m, nil
		}
		if key.Matches(msg, m.keys.CategoryMgr.Edit) {
			// Save edits
			if m.catMgrCursor < 0 || m.catMgrCursor >= len(m.Settings.General.Categories) {
				m.addLogEntry(LogStyleError.Render("✖ Invalid category selection"))
				return m, nil
			}

			name := strings.TrimSpace(m.catMgrInputs[0].Value())
			description := strings.TrimSpace(m.catMgrInputs[1].Value())
			pattern := strings.TrimSpace(m.catMgrInputs[2].Value())
			path := strings.TrimSpace(m.catMgrInputs[3].Value())

			if name == "" {
				m.addLogEntry(LogStyleError.Render("✖ Category name cannot be empty"))
				return m, nil
			}
			if pattern == "" {
				m.addLogEntry(LogStyleError.Render("✖ Category pattern cannot be empty"))
				return m, nil
			}
			if _, err := regexp.Compile(pattern); err != nil {
				m.addLogEntry(LogStyleError.Render(fmt.Sprintf("✖ Invalid category pattern: %v", err)))
				return m, nil
			}
			if path == "" {
				m.addLogEntry(LogStyleError.Render("✖ Category path cannot be empty"))
				return m, nil
			}

			target := &m.Settings.General.Categories[m.catMgrCursor]
			target.Name = name
			target.Description = description
			target.Pattern = pattern
			target.Path = filepath.Clean(path)

			m.catMgrEditing = false
			m.catMgrIsNew = false

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
		m.state = DashboardState
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Up) {
		if m.catMgrCursor > 0 {
			m.catMgrCursor--
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.CategoryMgr.Down) {
		if m.catMgrCursor < len(cats) { // len(cats) = "+Add" row
			m.catMgrCursor++
		}
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Toggle) {
		m.Settings.General.CategoryEnabled = !m.Settings.General.CategoryEnabled
		return m, nil
	}

	if key.Matches(msg, m.keys.CategoryMgr.Delete) {
		if m.catMgrCursor < len(cats) {
			m.Settings.General.Categories = append(
				m.Settings.General.Categories[:m.catMgrCursor],
				m.Settings.General.Categories[m.catMgrCursor+1:]...,
			)
			if m.catMgrCursor >= len(m.Settings.General.Categories) && m.catMgrCursor > 0 {
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
			m.catMgrInputs[0].Focus()
		} else {
			m.catMgrBeginAdd()
		}
		return m, nil
	}

	return m, nil
}
