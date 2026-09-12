package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/operationsview"
)

type jobReconnectProviderStub struct {
	snapshot operationsview.JobReconnectSnapshot
	err      error
	jobID    string
	after    uint64
}

func (stub *jobReconnectProviderStub) JobReconnect(_ context.Context, jobID string, after uint64) (operationsview.JobReconnectSnapshot, error) {
	stub.jobID = jobID
	stub.after = after
	return stub.snapshot, stub.err
}

func reconnectHTTPSnapshot(state operationsview.JobReconnectState) operationsview.JobReconnectSnapshot {
	now := time.Date(2026, 9, 12, 3, 50, 0, 0, time.UTC)
	snapshot := operationsview.JobReconnectSnapshot{
		ContractVersion:     operationsview.JobReconnectSnapshotContractVersion,
		State:               state,
		SourceAvailable:     true,
		JobID:               "job-31",
		JobVersion:          2,
		Status:              job.StatusRunning,
		Attempt:             1,
		MaxAttempts:         3,
		UpdatedAt:           now.Add(-time.Minute),
		Terminal:            false,
		AfterVersion:        1,
		ResumeVersion:       2,
		TimelineHeadVersion: 2,
		TimelineHeadStatus:  job.StatusRunning,
		TimelineHeadAttempt: 1,
		Events: []job.TimelineEntry{
			{
				JobID:      "job-31",
				Event:      job.TimelineAttemptStarted,
				Status:     job.StatusRunning,
				Attempt:    1,
				JobVersion: 2,
				OccurredAt: now.Add(-time.Minute),
			},
		},
		ReloadRequired:            false,
		EvaluatedAt:               now,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
	if state == operationsview.JobReconnectReloadRequired {
		snapshot.ReloadRequired = true
		snapshot.Events = []job.TimelineEntry{}
	}
	if state == operationsview.JobReconnectUnavailable {
		snapshot.SourceAvailable = false
		snapshot.ReloadRequired = true
		snapshot.TimelineHeadVersion = 0
		snapshot.TimelineHeadStatus = ""
		snapshot.TimelineHeadAttempt = 0
		snapshot.Events = []job.TimelineEntry{}
	}
	return snapshot
}

func TestJobReconnectHandlerReturnsCurrentSnapshot(t *testing.T) {
	provider := &jobReconnectProviderStub{snapshot: reconnectHTTPSnapshot(operationsview.JobReconnectCurrent)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ops/job-reconnect?job_id=job-31&after_version=1", nil)

	JobReconnectHandler(provider).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if provider.jobID != "job-31" || provider.after != 1 {
		t.Fatalf("provider args = (%q,%d)", provider.jobID, provider.after)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers missing: %#v", recorder.Header())
	}
	if !strings.Contains(recorder.Body.String(), `"execution_authorized":false`) {
		t.Fatalf("authority boundary not serialized: %s", recorder.Body.String())
	}
}

func TestJobReconnectHandlerSignalsReloadAndUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		snapshot   operationsview.JobReconnectSnapshot
		wantStatus int
	}{
		{name: "reload", snapshot: reconnectHTTPSnapshot(operationsview.JobReconnectReloadRequired), wantStatus: http.StatusConflict},
		{name: "unavailable", snapshot: reconnectHTTPSnapshot(operationsview.JobReconnectUnavailable), wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &jobReconnectProviderStub{snapshot: tt.snapshot}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/ops/job-reconnect?job_id=job-31&after_version=1", nil)
			JobReconnectHandler(provider).ServeHTTP(recorder, request)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestJobReconnectHandlerRejectsInvalidRequestAndProviderEvidence(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		url        string
		provider   *jobReconnectProviderStub
		wantStatus int
	}{
		{name: "method", method: http.MethodPost, url: "/ops/job-reconnect?job_id=job-31", provider: &jobReconnectProviderStub{}, wantStatus: http.StatusMethodNotAllowed},
		{name: "job id", method: http.MethodGet, url: "/ops/job-reconnect?job_id=%20job-31%20", provider: &jobReconnectProviderStub{}, wantStatus: http.StatusBadRequest},
		{name: "cursor", method: http.MethodGet, url: "/ops/job-reconnect?job_id=job-31&after_version=-1", provider: &jobReconnectProviderStub{}, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.url, nil)
			JobReconnectHandler(tt.provider).ServeHTTP(recorder, request)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ops/job-reconnect?job_id=job-31", nil)
	JobReconnectHandler(nil).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil provider status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	bad := reconnectHTTPSnapshot(operationsview.JobReconnectCurrent)
	bad.ExecutionAuthorized = true
	provider := &jobReconnectProviderStub{snapshot: bad}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ops/job-reconnect?job_id=job-31", nil)
	JobReconnectHandler(provider).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("authority-bearing provider evidence status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestJobReconnectHandlerMapsNotFoundWithoutLeakingBackendErrors(t *testing.T) {
	provider := &jobReconnectProviderStub{err: job.ErrNotFound}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ops/job-reconnect?job_id=job-31", nil)
	JobReconnectHandler(provider).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "job_not_found") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	provider.err = errors.New("database details must not leak")
	recorder = httptest.NewRecorder()
	JobReconnectHandler(provider).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || strings.Contains(recorder.Body.String(), "database details") {
		t.Fatalf("backend error leaked: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
