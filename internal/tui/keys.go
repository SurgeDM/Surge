package tui

import "charm.land/bubbles/v2/key"

// KeyMap defines the keybindings for the entire application
type KeyMap struct {
	Dashboard      DashboardKeyMap
	Input          InputKeyMap
	FilePicker     FilePickerKeyMap
	Duplicate      DuplicateKeyMap
	Extension      ExtensionKeyMap
	Settings       SettingsKeyMap
	SettingsEditor SettingsEditorKeyMap
	BatchConfirm   BatchConfirmKeyMap
	Update         UpdateKeyMap
	BugReport      BugReportKeyMap
	CategoryMgr    CategoryManagerKeyMap
	QuitConfirm    QuitConfirmKeyMap
}

// DashboardKeyMap defines keybindings for the main dashboard
type DashboardKeyMap struct {
	TabQueued      key.Binding
	TabActive      key.Binding
	TabDone        key.Binding
	NextTab        key.Binding
	Add            key.Binding
	BatchImport    key.Binding
	Search         key.Binding
	Pause          key.Binding
	Refresh        key.Binding
	Delete         key.Binding
	Settings       key.Binding
	Log            key.Binding
	ToggleHelp     key.Binding
	ReportBug      key.Binding
	OpenFile       key.Binding
	Quit           key.Binding
	ForceQuit      key.Binding
	CategoryFilter key.Binding
	// Navigation
	Up   key.Binding
	Down key.Binding
	// Log Navigation
	LogUp     key.Binding
	LogDown   key.Binding
	LogTop    key.Binding
	LogBottom key.Binding
	LogClose  key.Binding
}

// InputKeyMap defines keybindings for the add download input
type InputKeyMap struct {
	Tab    key.Binding
	Enter  key.Binding
	Esc    key.Binding
	Up     key.Binding
	Down   key.Binding
	Cancel key.Binding
}

// FilePickerKeyMap defines keybindings for the file picker
type FilePickerKeyMap struct {
	UseDir   key.Binding
	GotoHome key.Binding
	Back     key.Binding
	Forward  key.Binding
	Open     key.Binding
	Cancel   key.Binding
}

// DuplicateKeyMap defines keybindings for duplicate warning
type DuplicateKeyMap struct {
	Continue key.Binding
	Focus    key.Binding
	Cancel   key.Binding
}

// ExtensionKeyMap defines keybindings for extension confirmation
type ExtensionKeyMap struct {
	Confirm key.Binding
	Browse  key.Binding
	Next    key.Binding
	Prev    key.Binding
	Cancel  key.Binding
}

// SettingsKeyMap defines keybindings for the settings view
type SettingsKeyMap struct {
	Tab1    key.Binding
	Tab2    key.Binding
	Tab3    key.Binding
	Tab4    key.Binding
	Tab5    key.Binding
	NextTab key.Binding
	PrevTab key.Binding
	Browse  key.Binding
	Edit    key.Binding
	Up      key.Binding
	Down    key.Binding
	Reset   key.Binding
	Close   key.Binding
}

// SettingsEditorKeyMap defines keybindings for editing a setting
type SettingsEditorKeyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
}

// BatchConfirmKeyMap defines keybindings for batch import confirmation
type BatchConfirmKeyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
}

// UpdateKeyMap defines keybindings for update notification
type UpdateKeyMap struct {
	OpenGitHub  key.Binding
	IgnoreNow   key.Binding
	NeverRemind key.Binding
}

// BugReportKeyMap defines keybindings for selecting bug report target.
type BugReportKeyMap struct {
	Core      key.Binding
	Extension key.Binding
	Cancel    key.Binding
}

// QuitConfirmKeyMap defines keybindings for the quit confirmation modal
type QuitConfirmKeyMap struct {
	Left   key.Binding
	Right  key.Binding
	Yes    key.Binding
	No     key.Binding
	Select key.Binding
	Cancel key.Binding
}

// CategoryManagerKeyMap defines keybindings for the category manager
type CategoryManagerKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Edit   key.Binding
	Add    key.Binding
	Delete key.Binding
	Toggle key.Binding // toggle enable/disable
	Tab    key.Binding
	Close  key.Binding
}

// Keys contains all the keybindings for the application
var Keys = KeyMap{
	Dashboard: DashboardKeyMap{
		TabQueued: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "queued tab"),
		),
		TabActive: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "active tab"),
		),
		TabDone: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "done tab"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add download"),
		),
		BatchImport: key.NewBinding(
			key.WithKeys("b", "B"),
			key.WithHelp("b", "batch import"),
		),
		Search: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "search"),
		),
		Pause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause/resume"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh url"),
		),
		Delete: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "delete"),
		),
		Settings: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "settings"),
		),
		Log: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "toggle log"),
		),
		ToggleHelp: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "keybindings"),
		),
		ReportBug: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "report bug"),
		),
		OpenFile: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open file"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "ctrl+q"),
			key.WithHelp("ctrl+q", "quit"),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "force quit"),
		),
		CategoryFilter: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "category"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("\u2191/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("\u2193/j", "down"),
		),
		LogUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("\u2191/k", "scroll up"),
		),
		LogDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("\u2193/j", "scroll down"),
		),
		LogTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "top"),
		),
		LogBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		LogClose: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close log"),
		),
	},
	Input: InputKeyMap{
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "browse/next"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm/next"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("\u2191", "previous"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("\u2193", "next"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	},
	FilePicker: FilePickerKeyMap{
		UseDir: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "use current"),
		),
		GotoHome: key.NewBinding(
			key.WithKeys("h", "H"),
			key.WithHelp("h", "home"),
		),
		Back: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("\u2190", "back"),
		),
		Forward: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("\u2192", "open"),
		),
		Open: key.NewBinding(
			key.WithKeys("."),
			key.WithHelp(".", "select highlighted"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	},
	Duplicate: DuplicateKeyMap{
		Continue: key.NewBinding(
			key.WithKeys("c", "C"),
			key.WithHelp("c", "continue"),
		),
		Focus: key.NewBinding(
			key.WithKeys("f", "F"),
			key.WithHelp("f", "focus existing"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("x", "X", "esc"),
			key.WithHelp("x", "cancel"),
		),
	},
	Extension: ExtensionKeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Browse: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "browse path"),
		),
		Next: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("\u2193", "next field"),
		),
		Prev: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("\u2191", "prev field"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	},
	Settings: SettingsKeyMap{
		Tab1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "general"),
		),
		Tab2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "network"),
		),
		Tab3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "performance"),
		),
		Tab4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "categories"),
		),
		Tab5: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "extension"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("\u2192", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("\u2190", "prev tab"),
		),
		Browse: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "browse dir"),
		),
		Edit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "edit"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("\u2191/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("\u2193/j", "down"),
		),
		Reset: key.NewBinding(
			key.WithKeys("r", "R"),
			key.WithHelp("r", "reset"),
		),
		Close: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "save & close"),
		),
	},
	SettingsEditor: SettingsEditorKeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	},
	BatchConfirm: BatchConfirmKeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("y", "Y", "enter"),
			key.WithHelp("y", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("n", "N", "esc"),
			key.WithHelp("n", "cancel"),
		),
	},
	Update: UpdateKeyMap{
		OpenGitHub: key.NewBinding(
			key.WithKeys("o", "O", "enter"),
			key.WithHelp("o", "open on github"),
		),
		IgnoreNow: key.NewBinding(
			key.WithKeys("i", "I", "esc"),
			key.WithHelp("i", "ignore for now"),
		),
		NeverRemind: key.NewBinding(
			key.WithKeys("n", "N"),
			key.WithHelp("n", "never remind"),
		),
	},
	BugReport: BugReportKeyMap{
		Core: key.NewBinding(
			key.WithKeys("1", "c", "C"),
			key.WithHelp("1", "core report"),
		),
		Extension: key.NewBinding(
			key.WithKeys("2", "e", "E"),
			key.WithHelp("2", "extension report"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	},
	CategoryMgr: CategoryManagerKeyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("\u2191/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("\u2193/j", "down")),
		Edit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
		Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Delete: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete")),
		Toggle: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle")),
		Tab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		Close:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "save & close")),
	},
	QuitConfirm: QuitConfirmKeyMap{
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l", "tab"),
		),
		Yes: key.NewBinding(
			key.WithKeys("y", "Y"),
		),
		No: key.NewBinding(
			key.WithKeys("n", "N"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter", "space"),
			key.WithHelp("y/enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c", "ctrl+q"),
			key.WithHelp("n/esc", "cancel"),
		),
	},
}

// ShortHelp returns keybindings to show in the mini help view
func (k DashboardKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.ToggleHelp, k.ReportBug}
}

// FullHelp returns keybindings for the expanded help view
func (k DashboardKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.TabQueued, k.TabActive, k.TabDone, k.NextTab},
		{k.Add, k.BatchImport, k.Search, k.CategoryFilter, k.Pause, k.Refresh, k.Delete, k.Settings},
		{k.Log, k.OpenFile, k.ReportBug, k.Quit},
	}
}

func (k InputKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Enter, k.Esc}
}

func (k InputKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Tab, k.Enter, k.Esc}}
}

func (k FilePickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Back, k.Forward, k.UseDir, k.GotoHome, k.Open, k.Cancel}
}

func (k FilePickerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Back, k.Forward, k.UseDir, k.GotoHome, k.Open, k.Cancel}}
}

func (k DuplicateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Continue, k.Focus, k.Cancel}
}

func (k DuplicateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Continue, k.Focus, k.Cancel}}
}

func (k ExtensionKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Browse, k.Prev, k.Next, k.Confirm, k.Cancel}
}

func (k ExtensionKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Browse, k.Prev, k.Next, k.Confirm, k.Cancel}}
}

func (k SettingsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.PrevTab, k.NextTab, k.Edit, k.Reset, k.Close}
}

func (k SettingsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Tab5},
		{k.PrevTab, k.NextTab, k.Up, k.Down, k.Edit, k.Reset, k.Browse, k.Close},
	}
}

func (k SettingsEditorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Confirm, k.Cancel}
}

func (k SettingsEditorKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Confirm, k.Cancel}}
}

func (k BatchConfirmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Confirm, k.Cancel}
}

func (k BatchConfirmKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Confirm, k.Cancel}}
}

func (k UpdateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.OpenGitHub, k.IgnoreNow, k.NeverRemind}
}

func (k UpdateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.OpenGitHub, k.IgnoreNow, k.NeverRemind}}
}

func (k BugReportKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Core, k.Extension, k.Cancel}
}

func (k BugReportKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Core, k.Extension, k.Cancel}}
}

func (k CategoryManagerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Edit, k.Add, k.Delete, k.Tab, k.Toggle, k.Close}
}

func (k CategoryManagerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Edit, k.Add, k.Delete, k.Tab, k.Toggle, k.Close}}
}

func (k QuitConfirmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Cancel}
}

func (k QuitConfirmKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Select, k.Cancel}}
}
