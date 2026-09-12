package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSplitHandlerRoutesAuditReadAndExportWithoutCoreFallback(t *testing.T) {
	identityCalls := 0
	exportCalls := 0
	coreCalls := 0
	handler := splitHandler{
		core: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			coreCalls++
			w.WriteHeader(http.StatusTeapot)
		}),
		identity: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			identityCalls++
			w.WriteHeader(http.StatusNoContent)
		}),
		auditExport: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			exportCalls++
			w.WriteHeader(http.StatusAccepted)
		}),
	}

	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?limit=10", nil))
	if read.Code != http.StatusNoContent {
		t.Fatalf("audit read status = %d, want %d", read.Code, http.StatusNoContent)
	}

	export := httptest.NewRecorder()
	handler.ServeHTTP(export, httptest.NewRequest(http.MethodGet, "/api/v1/audit/events/export?limit=10", nil))
	if export.Code != http.StatusAccepted {
		t.Fatalf("audit export status = %d, want %d", export.Code, http.StatusAccepted)
	}

	if identityCalls != 1 || exportCalls != 1 || coreCalls != 0 {
		t.Fatalf("identityCalls=%d exportCalls=%d coreCalls=%d, want 1/1/0", identityCalls, exportCalls, coreCalls)
	}
}

func TestSplitHandlerDoesNotCaptureAuditLookalikePath(t *testing.T) {
	identityCalls := 0
	exportCalls := 0
	coreCalls := 0
	handler := splitHandler{
		core: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			coreCalls++
			w.WriteHeader(http.StatusTeapot)
		}),
		identity: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			identityCalls++
			w.WriteHeader(http.StatusNoContent)
		}),
		auditExport: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			exportCalls++
			w.WriteHeader(http.StatusAccepted)
		}),
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audit-export", nil))
	if recorder.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTeapot)
	}
	if identityCalls != 0 || exportCalls != 0 || coreCalls != 1 {
		t.Fatalf("identityCalls=%d exportCalls=%d coreCalls=%d, want 0/0/1", identityCalls, exportCalls, coreCalls)
	}
}
