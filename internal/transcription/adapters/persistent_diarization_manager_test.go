package adapters

import "testing"

func TestPersistentDiarizationWorkerArgsDefaultVRAMReservation(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		params  map[string]interface{}
	}{
		{
			name:    "pyannote",
			modelID: PersistentDiarizationModelPyAnnote,
			params: map[string]interface{}{
				"hf_token": "test-token",
			},
		},
		{
			name:    "sortformer",
			modelID: PersistentDiarizationModelSortformer,
			params:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := persistentDiarizationWorkerArgs(tt.modelID, "/tmp/env", "/tmp/worker.py", tt.params)
			if err != nil {
				t.Fatalf("expected worker args, got error: %v", err)
			}

			if !argsHaveFlagValue(args, "--reserve-vram-mb", "2700") {
				t.Fatalf("expected default 2700 MiB reservation in args: %#v", args)
			}
		})
	}
}

func TestPersistentDiarizationWorkerArgsOverrideVRAMReservation(t *testing.T) {
	args, err := persistentDiarizationWorkerArgs(PersistentDiarizationModelSortformer, "/tmp/env", "/tmp/worker.py", map[string]interface{}{
		"reserve_vram_mb": 1024,
	})
	if err != nil {
		t.Fatalf("expected worker args, got error: %v", err)
	}

	if !argsHaveFlagValue(args, "--reserve-vram-mb", "1024") {
		t.Fatalf("expected overridden 1024 MiB reservation in args: %#v", args)
	}
}

func argsHaveFlagValue(args []string, flag string, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
