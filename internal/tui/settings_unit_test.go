package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettingsUnitConversion(t *testing.T) {
	m := RootModel{
		Settings: config.DefaultSettings(),
	}

	tests := []struct {
		name          string
		category      string
		key           string
		typ           string
		internalValue interface{}
		uiInput       string
		expectedValue interface{}
	}{
		{
			name:          "MinChunkSize MB Conversion",
			category:      "Network",
			key:           "min_chunk_size",
			typ:           "int64",
			internalValue: int64(4 * config.MB),
			uiInput:       "4.0",
			expectedValue: int64(4 * config.MB),
		},
		{
			name:          "WorkerBufferSize KB Conversion",
			category:      "Network",
			key:           "worker_buffer_size",
			typ:           "int",
			internalValue: 1024 * config.KB,
			uiInput:       "1024",
			expectedValue: 1024 * config.KB,
		},
		{
			name:          "SlowWorkerGracePeriod seconds Conversion",
			category:      "Performance",
			key:           "slow_worker_grace_period",
			typ:           "duration",
			internalValue: 10 * time.Second,
			uiInput:       "10",
			expectedValue: 10 * time.Second,
		},
		{
			name:          "StallTimeout seconds Conversion",
			category:      "Performance",
			key:           "stall_timeout",
			typ:           "duration",
			internalValue: 5 * time.Second,
			uiInput:       "5",
			expectedValue: 5 * time.Second,
		},
		{
			name:          "SlowWorkerThreshold float Comparison",
			category:      "Performance",
			key:           "slow_worker_threshold",
			typ:           "float64",
			internalValue: 0.35,
			uiInput:       "0.35",
			expectedValue: 0.35,
		},
		{
			name:          "SpeedEmaAlpha float Comparison",
			category:      "Performance",
			key:           "speed_ema_alpha",
			typ:           "float64",
			internalValue: 0.5,
			uiInput:       "0.50",
			expectedValue: 0.5,
		},
		{
			name:          "MaxTaskRetries int Comparison",
			category:      "Performance",
			key:           "max_task_retries",
			typ:           "int",
			internalValue: 5,
			uiInput:       "5",
			expectedValue: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Test Internal -> UI String (formatSettingValueForEdit)
			gotUI := formatSettingValueForEdit(tt.internalValue, tt.typ, tt.key, false)
			// For floats like 4.0 vs 4, we normalize by parsing back
			if gotUI != tt.uiInput {
				t.Errorf("%s: formatSettingValueForEdit() = %q, want %q", tt.name, gotUI, tt.uiInput)
			}

			// 2. Test UI String -> Internal (setSettingValue)
			err := m.setSettingValue(tt.category, tt.key, tt.uiInput)
			if err != nil {
				t.Fatalf("%s: setSettingValue() returned error: %v", tt.name, err)
			}

			// Read back the value using reflection similar to how the app does
			values := m.getSettingsValues(tt.category)
			gotInternal := values[tt.key]

			if !reflect.DeepEqual(gotInternal, tt.expectedValue) {
				t.Errorf("%s: Value after setSettingValue() = %v (%T), want %v (%T)",
					tt.name, gotInternal, gotInternal, tt.expectedValue, tt.expectedValue)
			}
		})
	}
}
