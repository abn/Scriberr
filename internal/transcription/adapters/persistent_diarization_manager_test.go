package adapters

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

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

func TestWithTemporaryModelRestoresResidentModelAndVRAMReservation(t *testing.T) {
	manager, events := newTestPersistentDiarizationManager()
	residentParams := map[string]interface{}{
		"hf_token":        "test-token",
		"model":           "pyannote/test-model",
		"device":          "cuda",
		"reserve_vram_mb": 1536,
	}

	if _, err := manager.Load(context.Background(), PersistentDiarizationModelPyAnnote, residentParams); err != nil {
		t.Fatalf("load resident model: %v", err)
	}

	err := manager.WithTemporaryModel(
		context.Background(),
		PersistentDiarizationModelSortformer,
		map[string]interface{}{"device": "cuda", "reserve_vram_mb": 512},
		func() error {
			*events = append(*events, "run:"+manager.Status().ModelID)
			if !manager.IsModelLoaded(PersistentDiarizationModelSortformer) {
				t.Fatal("expected Sortformer to be resident during the temporary task")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("temporary model task: %v", err)
	}

	wantEvents := []string{
		"load:pyannote:1536",
		"unload:pyannote",
		"load:sortformer:1536",
		"run:sortformer",
		"unload:sortformer",
		"load:pyannote:1536",
	}
	if !reflect.DeepEqual(*events, wantEvents) {
		t.Fatalf("unexpected model lifecycle:\n got: %#v\nwant: %#v", *events, wantEvents)
	}

	if !manager.IsModelLoaded(PersistentDiarizationModelPyAnnote) {
		t.Fatal("expected PyAnnote to be restored after the temporary task")
	}
	if got := manager.loadParams["hf_token"]; got != "test-token" {
		t.Fatalf("expected original private load parameters to be restored, got token %#v", got)
	}
	if got := intParam(manager.loadParams, "reserve_vram_mb", 0); got != 1536 {
		t.Fatalf("expected original VRAM reservation to be restored, got %d MiB", got)
	}
}

func TestWithTemporaryModelRestoresResidentModelAfterTaskFailure(t *testing.T) {
	manager, events := newTestPersistentDiarizationManager()
	if _, err := manager.Load(context.Background(), PersistentDiarizationModelPyAnnote, map[string]interface{}{
		"hf_token": "test-token",
	}); err != nil {
		t.Fatalf("load resident model: %v", err)
	}

	taskErr := errors.New("diarization failed")
	err := manager.WithTemporaryModel(
		context.Background(),
		PersistentDiarizationModelSortformer,
		nil,
		func() error {
			*events = append(*events, "run:"+manager.Status().ModelID)
			return taskErr
		},
	)
	if !errors.Is(err, taskErr) {
		t.Fatalf("expected task error, got %v", err)
	}
	if !manager.IsModelLoaded(PersistentDiarizationModelPyAnnote) {
		t.Fatal("expected PyAnnote to be restored after task failure")
	}

	wantEvents := []string{
		"load:pyannote:2700",
		"unload:pyannote",
		"load:sortformer:2700",
		"run:sortformer",
		"unload:sortformer",
		"load:pyannote:2700",
	}
	if !reflect.DeepEqual(*events, wantEvents) {
		t.Fatalf("unexpected failure lifecycle:\n got: %#v\nwant: %#v", *events, wantEvents)
	}
}

func newTestPersistentDiarizationManager() (*PersistentDiarizationManager, *[]string) {
	manager := NewPersistentDiarizationManager()
	events := &[]string{}
	manager.SetEnvironment(PersistentDiarizationModelPyAnnote, "/tmp/pyannote")
	manager.SetEnvironment(PersistentDiarizationModelSortformer, "/tmp/sortformer")
	manager.startWorker = func(_ context.Context, modelID, _ string, params map[string]interface{}) (*persistentDiarizationWorker, error) {
		*events = append(*events, fmt.Sprintf("load:%s:%d", modelID, intParam(params, "reserve_vram_mb", PersistentDiarizationDefaultVRAMReserveMB)))
		return &persistentDiarizationWorker{modelID: modelID}, nil
	}
	manager.stopWorker = func(_ context.Context, worker *persistentDiarizationWorker) error {
		*events = append(*events, "unload:"+worker.modelID)
		return nil
	}
	return manager, events
}

func argsHaveFlagValue(args []string, flag string, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
