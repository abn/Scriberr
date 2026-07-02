package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"scriberr/pkg/logger"
)

const (
	PersistentDiarizationStateUnloaded  = "unloaded"
	PersistentDiarizationStateLoading   = "loading"
	PersistentDiarizationStateLoaded    = "loaded"
	PersistentDiarizationStateUnloading = "unloading"
	PersistentDiarizationStateFailed    = "failed"

	PersistentDiarizationModelPyAnnote   = "pyannote"
	PersistentDiarizationModelSortformer = "sortformer"

	PersistentDiarizationDefaultVRAMReserveMB = 2700
	persistentDiarizationRestoreTimeout       = 10 * time.Minute
)

var errPersistentWorkerStopped = errors.New("persistent diarization worker stopped")

// PersistentDiarizationStatus describes the resident diarization model, if any.
type PersistentDiarizationStatus struct {
	State       string     `json:"state"`
	Loaded      bool       `json:"loaded"`
	ModelID     string     `json:"model_id,omitempty"`
	DisplayName string     `json:"display_name,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	PID         int        `json:"pid,omitempty"`
}

// PersistentDiarizationManager owns the single resident diarization process.
type PersistentDiarizationManager struct {
	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	envPaths    map[string]string
	worker      *persistentDiarizationWorker
	status      PersistentDiarizationStatus
	loadParams  map[string]interface{}
	startWorker func(context.Context, string, string, map[string]interface{}) (*persistentDiarizationWorker, error)
	stopWorker  func(context.Context, *persistentDiarizationWorker) error
}

var persistentDiarizationManager = NewPersistentDiarizationManager()

// NewPersistentDiarizationManager creates a manager with no loaded model.
func NewPersistentDiarizationManager() *PersistentDiarizationManager {
	return &PersistentDiarizationManager{
		envPaths:    make(map[string]string),
		startWorker: startPersistentDiarizationWorker,
		stopWorker: func(ctx context.Context, worker *persistentDiarizationWorker) error {
			return worker.Stop(ctx)
		},
		status: PersistentDiarizationStatus{
			State: PersistentDiarizationStateUnloaded,
		},
	}
}

// GetPersistentDiarizationManager returns the process-wide resident model manager.
func GetPersistentDiarizationManager() *PersistentDiarizationManager {
	return persistentDiarizationManager
}

// SetEnvironment registers the UV project path used by a diarization model.
func (m *PersistentDiarizationManager) SetEnvironment(modelID, envPath string) {
	modelID = normalizePersistentDiarizationModel(modelID)
	if modelID == "" || envPath == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.envPaths[modelID] = envPath
}

// Status returns a copy of the current persistent worker state.
func (m *PersistentDiarizationManager) Status() PersistentDiarizationStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// IsModelLoaded reports whether the requested model is currently resident.
func (m *PersistentDiarizationManager) IsModelLoaded(modelID string) bool {
	modelID = normalizePersistentDiarizationModel(modelID)

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.worker != nil && m.status.Loaded && m.status.ModelID == modelID
}

// Load starts a resident diarization worker and waits until the model is loaded.
func (m *PersistentDiarizationManager) Load(ctx context.Context, modelID string, params map[string]interface{}) (PersistentDiarizationStatus, error) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if err := ctx.Err(); err != nil {
		return m.Status(), fmt.Errorf("loading persistent diarization model cancelled: %w", err)
	}

	return m.load(ctx, modelID, params)
}

func (m *PersistentDiarizationManager) load(ctx context.Context, modelID string, params map[string]interface{}) (PersistentDiarizationStatus, error) {
	modelID = normalizePersistentDiarizationModel(modelID)
	if modelID == "" {
		return m.Status(), fmt.Errorf("unsupported diarization model")
	}

	m.mu.Lock()
	if m.status.State == PersistentDiarizationStateLoading || m.status.State == PersistentDiarizationStateUnloading {
		status := m.status
		m.mu.Unlock()
		return status, fmt.Errorf("diarization worker is currently %s", status.State)
	}
	if m.worker != nil && m.status.Loaded {
		status := m.status
		m.mu.Unlock()
		if status.ModelID == modelID {
			return status, nil
		}
		return status, fmt.Errorf("%s is already loaded; unload it before loading %s", status.DisplayName, persistentDiarizationDisplayName(modelID))
	}

	envPath := m.envPaths[modelID]
	startedAt := time.Now()
	m.status = PersistentDiarizationStatus{
		State:       PersistentDiarizationStateLoading,
		ModelID:     modelID,
		DisplayName: persistentDiarizationDisplayName(modelID),
		StartedAt:   &startedAt,
	}
	m.mu.Unlock()

	worker, err := m.startWorker(ctx, modelID, envPath, params)
	if err != nil {
		m.setFailed(modelID, startedAt, err)
		return m.Status(), err
	}

	m.mu.Lock()
	m.worker = worker
	m.loadParams = cloneDiarizationParams(params)
	m.status = PersistentDiarizationStatus{
		State:       PersistentDiarizationStateLoaded,
		Loaded:      true,
		ModelID:     modelID,
		DisplayName: persistentDiarizationDisplayName(modelID),
		StartedAt:   &startedAt,
		PID:         worker.pid(),
	}
	status := m.status
	m.mu.Unlock()

	logger.Info("Persistent diarization model loaded", "model_id", modelID, "pid", worker.pid())
	return status, nil
}

// Unload stops the resident diarization worker, if one is running.
func (m *PersistentDiarizationManager) Unload(ctx context.Context) (PersistentDiarizationStatus, error) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if err := ctx.Err(); err != nil {
		return m.Status(), fmt.Errorf("unloading persistent diarization model cancelled: %w", err)
	}

	return m.unload(ctx)
}

func (m *PersistentDiarizationManager) unload(ctx context.Context) (PersistentDiarizationStatus, error) {
	m.mu.Lock()
	if m.worker == nil {
		m.loadParams = nil
		m.status = PersistentDiarizationStatus{State: PersistentDiarizationStateUnloaded}
		status := m.status
		m.mu.Unlock()
		return status, nil
	}

	worker := m.worker
	modelID := m.status.ModelID
	startedAt := m.status.StartedAt
	m.status.State = PersistentDiarizationStateUnloading
	m.status.Loaded = false
	status := m.status
	m.mu.Unlock()

	if err := m.stopWorker(ctx, worker); err != nil {
		m.setFailed(modelID, derefTime(startedAt), err)
		return m.Status(), err
	}

	m.mu.Lock()
	if m.worker == worker {
		m.worker = nil
		m.loadParams = nil
		m.status = PersistentDiarizationStatus{State: PersistentDiarizationStateUnloaded}
		status = m.status
	}
	m.mu.Unlock()

	logger.Info("Persistent diarization model unloaded", "model_id", modelID)
	return status, nil
}

// WithTemporaryModel runs diarization with modelID, temporarily replacing and
// then restoring a different resident model when necessary. The temporary
// worker inherits the resident worker's VRAM reservation size.
func (m *PersistentDiarizationManager) WithTemporaryModel(
	ctx context.Context,
	modelID string,
	params map[string]interface{},
	run func() error,
) (resultErr error) {
	modelID = normalizePersistentDiarizationModel(modelID)
	if modelID == "" {
		return fmt.Errorf("unsupported diarization model")
	}
	if run == nil {
		return fmt.Errorf("temporary diarization callback is required")
	}

	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("temporary diarization model swap cancelled: %w", err)
	}

	m.mu.RLock()
	residentLoaded := m.worker != nil && m.status.Loaded
	residentModelID := m.status.ModelID
	residentParams := cloneDiarizationParams(m.loadParams)
	m.mu.RUnlock()

	if !residentLoaded || residentModelID == modelID {
		return run()
	}

	temporaryParams := cloneDiarizationParams(params)
	temporaryParams["reserve_vram_mb"] = intParam(
		residentParams,
		"reserve_vram_mb",
		PersistentDiarizationDefaultVRAMReserveMB,
	)

	logger.Info("Temporarily swapping persistent diarization model",
		"resident_model_id", residentModelID,
		"temporary_model_id", modelID,
		"reserve_vram_mb", temporaryParams["reserve_vram_mb"])

	if _, err := m.unload(ctx); err != nil {
		restoreErr := m.restoreResidentModel(residentModelID, residentParams)
		return errors.Join(fmt.Errorf("failed to unload resident diarization model: %w", err), restoreErr)
	}

	if _, err := m.load(ctx, modelID, temporaryParams); err != nil {
		restoreErr := m.restoreResidentModel(residentModelID, residentParams)
		return errors.Join(fmt.Errorf("failed to load temporary diarization model: %w", err), restoreErr)
	}

	defer func() {
		restoreErr := m.restoreResidentModel(residentModelID, residentParams)
		resultErr = errors.Join(resultErr, restoreErr)
	}()

	return run()
}

func (m *PersistentDiarizationManager) restoreResidentModel(modelID string, params map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), persistentDiarizationRestoreTimeout)
	defer cancel()

	var restoreErr error
	if _, err := m.unload(ctx); err != nil {
		restoreErr = errors.Join(restoreErr, fmt.Errorf("failed to unload temporary diarization model: %w", err))
	}
	if _, err := m.load(ctx, modelID, params); err != nil {
		restoreErr = errors.Join(restoreErr, fmt.Errorf("failed to restore resident diarization model: %w", err))
		return restoreErr
	}

	logger.Info("Restored persistent diarization model", "model_id", modelID)
	return restoreErr
}

// Diarize runs a request through the resident model. The caller still owns output parsing.
func (m *PersistentDiarizationManager) Diarize(ctx context.Context, modelID string, payload map[string]interface{}) error {
	modelID = normalizePersistentDiarizationModel(modelID)

	m.mu.RLock()
	worker := m.worker
	loaded := worker != nil && m.status.Loaded && m.status.ModelID == modelID
	m.mu.RUnlock()

	if !loaded {
		return fmt.Errorf("persistent %s diarization model is not loaded", persistentDiarizationDisplayName(modelID))
	}

	err := worker.Request(ctx, payload)
	if errors.Is(err, errPersistentWorkerStopped) {
		m.mu.Lock()
		if m.worker == worker {
			m.worker = nil
			m.loadParams = nil
			m.status = PersistentDiarizationStatus{
				State:       PersistentDiarizationStateFailed,
				Loaded:      false,
				ModelID:     modelID,
				DisplayName: persistentDiarizationDisplayName(modelID),
				Error:       err.Error(),
			}
		}
		m.mu.Unlock()
	}
	return err
}

func (m *PersistentDiarizationManager) setFailed(modelID string, startedAt time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worker = nil
	m.loadParams = nil
	m.status = PersistentDiarizationStatus{
		State:       PersistentDiarizationStateFailed,
		ModelID:     modelID,
		DisplayName: persistentDiarizationDisplayName(modelID),
		Error:       err.Error(),
		StartedAt:   &startedAt,
	}
}

func cloneDiarizationParams(params map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

type persistentDiarizationWorker struct {
	modelID     string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	responses   chan workerProtocolMessage
	ready       chan workerProtocolMessage
	done        chan error
	requestMu   sync.Mutex
	protocolMu  sync.Mutex
	stopped     bool
	protocolEnc *json.Encoder
}

type workerProtocolMessage struct {
	Type    string `json:"type,omitempty"`
	ID      string `json:"id,omitempty"`
	OK      bool   `json:"ok,omitempty"`
	Error   string `json:"error,omitempty"`
	ModelID string `json:"model_id,omitempty"`
}

func startPersistentDiarizationWorker(ctx context.Context, modelID, envPath string, params map[string]interface{}) (*persistentDiarizationWorker, error) {
	if envPath == "" {
		return nil, fmt.Errorf("environment path for %s is not registered", persistentDiarizationDisplayName(modelID))
	}

	scriptPath := filepath.Join(envPath, persistentDiarizationWorkerScript(modelID))
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("persistent worker script not found at %s: %w", scriptPath, err)
	}

	args, err := persistentDiarizationWorkerArgs(modelID, envPath, scriptPath, params)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("uv", args...)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open worker stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open worker stderr: %w", err)
	}

	worker := &persistentDiarizationWorker{
		modelID:     modelID,
		cmd:         cmd,
		stdin:       stdin,
		responses:   make(chan workerProtocolMessage, 4),
		ready:       make(chan workerProtocolMessage, 1),
		done:        make(chan error, 1),
		protocolEnc: json.NewEncoder(stdin),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start persistent diarization worker: %w", err)
	}

	go worker.readStdout(stdout)
	go worker.readStderr(stderr)
	go func() {
		worker.done <- cmd.Wait()
		close(worker.done)
	}()

	select {
	case msg := <-worker.ready:
		if msg.Type == "ready" && msg.OK {
			return worker, nil
		}
		_ = worker.Stop(context.Background())
		if msg.Error != "" {
			return nil, fmt.Errorf("persistent diarization worker failed to load: %s", msg.Error)
		}
		return nil, fmt.Errorf("persistent diarization worker failed to load")
	case err := <-worker.done:
		if err != nil {
			return nil, fmt.Errorf("persistent diarization worker exited during load: %w", err)
		}
		return nil, fmt.Errorf("persistent diarization worker exited during load")
	case <-ctx.Done():
		_ = worker.Stop(context.Background())
		return nil, fmt.Errorf("loading persistent diarization worker cancelled: %w", ctx.Err())
	}
}

func persistentDiarizationWorkerArgs(modelID, envPath, scriptPath string, params map[string]interface{}) ([]string, error) {
	args := []string{"run", "--native-tls", "--project", envPath, "python", scriptPath}
	device := stringParam(params, "device", "auto")
	reserveVRAMMB := intParam(params, "reserve_vram_mb", PersistentDiarizationDefaultVRAMReserveMB)

	switch modelID {
	case PersistentDiarizationModelPyAnnote:
		hfToken := stringParam(params, "hf_token", "")
		if hfToken == "" {
			hfToken = os.Getenv("HF_TOKEN")
		}
		if hfToken == "" {
			return nil, fmt.Errorf("HuggingFace token is required to load PyAnnote. Set HF_TOKEN before loading the resident model")
		}
		args = append(args,
			"--hf-token", hfToken,
			"--model", stringParam(params, "model", "pyannote/speaker-diarization-community-1"),
			"--device", device,
			"--reserve-vram-mb", strconv.Itoa(reserveVRAMMB),
		)
	case PersistentDiarizationModelSortformer:
		args = append(args,
			"--device", device,
			"--reserve-vram-mb", strconv.Itoa(reserveVRAMMB),
		)
	default:
		return nil, fmt.Errorf("unsupported persistent diarization model: %s", modelID)
	}

	return args, nil
}

func (w *persistentDiarizationWorker) Request(ctx context.Context, payload map[string]interface{}) error {
	w.requestMu.Lock()
	defer w.requestMu.Unlock()

	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
	request := make(map[string]interface{}, len(payload)+2)
	for key, value := range payload {
		request[key] = value
	}
	request["id"] = requestID
	request["action"] = "diarize"

	w.protocolMu.Lock()
	if w.stopped {
		w.protocolMu.Unlock()
		return errPersistentWorkerStopped
	}
	if err := w.protocolEnc.Encode(request); err != nil {
		w.protocolMu.Unlock()
		return fmt.Errorf("failed to send diarization request: %w", err)
	}
	w.protocolMu.Unlock()

	for {
		select {
		case msg := <-w.responses:
			if msg.ID != requestID {
				logger.Warn("Ignoring stale persistent diarization response", "expected_id", requestID, "response_id", msg.ID)
				continue
			}
			if !msg.OK {
				if msg.Error == "" {
					msg.Error = "worker returned an unknown error"
				}
				return fmt.Errorf("persistent diarization failed: %s", msg.Error)
			}
			return nil
		case err := <-w.done:
			if err != nil {
				return fmt.Errorf("%w: %v", errPersistentWorkerStopped, err)
			}
			return fmt.Errorf("%w: process exited", errPersistentWorkerStopped)
		case <-ctx.Done():
			w.forceStop()
			return fmt.Errorf("%w: request cancelled: %v", errPersistentWorkerStopped, ctx.Err())
		}
	}
}

func (w *persistentDiarizationWorker) Stop(ctx context.Context) error {
	w.protocolMu.Lock()
	if w.stopped {
		w.protocolMu.Unlock()
		return nil
	}
	w.stopped = true
	err := w.protocolEnc.Encode(map[string]interface{}{
		"id":     fmt.Sprintf("shutdown-%d", time.Now().UnixNano()),
		"action": "shutdown",
	})
	w.protocolMu.Unlock()
	if err != nil {
		w.forceStop()
		return fmt.Errorf("failed to send persistent diarization worker shutdown request: %w", err)
	}

	killed := false
	select {
	case err := <-w.done:
		return err
	case <-time.After(5 * time.Second):
		w.forceStop()
		killed = true
	case <-ctx.Done():
		w.forceStop()
		return ctx.Err()
	}

	select {
	case err := <-w.done:
		if killed {
			return nil
		}
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("persistent diarization worker did not exit after kill")
	}
}

func (w *persistentDiarizationWorker) forceStop() {
	w.protocolMu.Lock()
	w.stopped = true
	w.protocolMu.Unlock()

	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

func (w *persistentDiarizationWorker) readStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg workerProtocolMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			logger.Warn("Ignoring non-protocol persistent diarization output", "model_id", w.modelID, "line", line)
			continue
		}

		if msg.Type == "ready" || (msg.Type == "error" && msg.ID == "") {
			select {
			case w.ready <- msg:
			default:
				logger.Warn("Dropping duplicate persistent diarization ready message", "model_id", w.modelID)
			}
			continue
		}

		select {
		case w.responses <- msg:
		default:
			logger.Warn("Dropping persistent diarization response because response channel is full", "model_id", w.modelID, "id", msg.ID)
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Warn("Persistent diarization stdout scanner failed", "model_id", w.modelID, "error", err)
	}
}

func (w *persistentDiarizationWorker) readStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		logger.Info("Persistent diarization worker", "model_id", w.modelID, "message", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("Persistent diarization stderr scanner failed", "model_id", w.modelID, "error", err)
	}
}

func (w *persistentDiarizationWorker) pid() int {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return 0
	}
	return w.cmd.Process.Pid
}

func normalizePersistentDiarizationModel(modelID string) string {
	switch strings.TrimSpace(strings.ToLower(modelID)) {
	case "pyannote", "pyannote/speaker-diarization-3.1", "pyannote/speaker-diarization-community-1":
		return PersistentDiarizationModelPyAnnote
	case "sortformer", "nvidia_sortformer", "nvidia/diar_streaming_sortformer_4spk-v2":
		return PersistentDiarizationModelSortformer
	default:
		return ""
	}
}

func persistentDiarizationDisplayName(modelID string) string {
	switch normalizePersistentDiarizationModel(modelID) {
	case PersistentDiarizationModelPyAnnote:
		return "PyAnnote"
	case PersistentDiarizationModelSortformer:
		return "NVIDIA Sortformer"
	default:
		return modelID
	}
}

func persistentDiarizationWorkerScript(modelID string) string {
	switch modelID {
	case PersistentDiarizationModelPyAnnote:
		return "pyannote_worker.py"
	case PersistentDiarizationModelSortformer:
		return "sortformer_worker.py"
	default:
		return ""
	}
}

func stringParam(params map[string]interface{}, key, fallback string) string {
	if params == nil {
		return fallback
	}
	if value, ok := params[key]; ok {
		if str, ok := value.(string); ok && str != "" {
			return str
		}
	}
	return fallback
}

func intParam(params map[string]interface{}, key string, fallback int) int {
	if params == nil {
		return fallback
	}
	switch value := params[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Now()
	}
	return *value
}
