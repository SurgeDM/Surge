package tui

import (
	"testing"
)

func TestTabFiltering(t *testing.T) {
	tests := []struct {
		name          string
		activeTab     int
		downloads     []*DownloadModel
		expectedCount int
	}{
		{
			name:      "Active Tab shows non-paused with speed",
			activeTab: TabActive,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 1024, done: false, paused: false},
				{ID: "2", Speed: 0, done: false, paused: true},
			},
			expectedCount: 1,
		},
		{
			name:      "Active Tab shows non-paused with connections",
			activeTab: TabActive,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 0, Connections: 1, done: false, paused: false},
			},
			expectedCount: 1,
		},
		{
			name:      "Active Tab shows resuming downloads",
			activeTab: TabActive,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 0, done: false, paused: false, resuming: true},
			},
			expectedCount: 1,
		},
		{
			name:      "Active Tab excludes paused even with speed",
			activeTab: TabActive,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 1024, done: false, paused: true},
			},
			expectedCount: 0,
		},
		{
			name:      "Queued Tab includes paused downloads",
			activeTab: TabQueued,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 0, done: false, paused: true},
			},
			expectedCount: 1,
		},
		{
			name:      "Queued Tab includes downloads with 0 speed and 0 connections",
			activeTab: TabQueued,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 0, Connections: 0, done: false, paused: false},
			},
			expectedCount: 1,
		},
		{
			name:      "Queued Tab excludes truly active downloads",
			activeTab: TabQueued,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 1024, done: false, paused: false},
			},
			expectedCount: 0,
		},
		{
			name:      "Active Tab excludes pausing downloads",
			activeTab: TabActive,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 1024, done: false, pausing: true},
			},
			expectedCount: 0,
		},
		{
			name:      "Queued Tab includes pausing downloads",
			activeTab: TabQueued,
			downloads: []*DownloadModel{
				{ID: "1", Speed: 1024, done: false, pausing: true},
			},
			expectedCount: 1,
		},
		{
			name:      "Done Tab shows completed downloads",
			activeTab: TabDone,
			downloads: []*DownloadModel{
				{ID: "1", done: true},
				{ID: "2", done: false},
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := RootModel{
				activeTab: tt.activeTab,
				downloads: tt.downloads,
			}
			filtered := m.getFilteredDownloads()
			if len(filtered) != tt.expectedCount {
				t.Errorf("getFilteredDownloads() length = %v, want %v", len(filtered), tt.expectedCount)
			}
		})
	}
}

func TestComputeViewStatsConsistency(t *testing.T) {
	downloads := []*DownloadModel{
		{ID: "active-speed", Speed: 1024, done: false, paused: false},
		{ID: "active-conns", Speed: 0, Connections: 1, done: false, paused: false},
		{ID: "active-resuming", Speed: 0, done: false, paused: false, resuming: true},
		{ID: "paused", Speed: 0, done: false, paused: true},
		{ID: "pausing", Speed: 1024, done: false, pausing: true},
		{ID: "queued", Speed: 0, Connections: 0, done: false},
		{ID: "done", done: true},
	}

	m := RootModel{
		downloads: downloads,
	}

	stats := m.ComputeViewStats()

	// Verify Active Tab
	m.activeTab = TabActive
	if stats.ActiveCount != len(m.getFilteredDownloads()) {
		t.Errorf("ActiveCount (%d) does not match getFilteredDownloads (%d)", stats.ActiveCount, len(m.getFilteredDownloads()))
	}

	// Verify Queued Tab
	m.activeTab = TabQueued
	if stats.QueuedCount != len(m.getFilteredDownloads()) {
		t.Errorf("QueuedCount (%d) does not match getFilteredDownloads (%d)", stats.QueuedCount, len(m.getFilteredDownloads()))
	}

	// Verify Done Tab
	m.activeTab = TabDone
	if stats.DownloadedCount != len(m.getFilteredDownloads()) {
		t.Errorf("DownloadedCount (%d) does not match getFilteredDownloads (%d)", stats.DownloadedCount, len(m.getFilteredDownloads()))
	}
}
