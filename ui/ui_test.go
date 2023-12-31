package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_setContentTypeMiddleware(t *testing.T) {
	handler := setContentTypeMiddleware(
		http.FileServer(http.Dir("./testdata")))
	request, _ := http.NewRequest(http.MethodGet, "/test.css.gz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	assertSliceEqual(t, []string{"text/css"}, response.Header["Content-Type"])
	assertSliceEqual(t, []string{"gzip"}, response.Header["Content-Encoding"])
}

func assertEqual[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()
	if expected == actual {
		return
	}
	t.Errorf("expected (%+v) is not equal to actual (%+v)", expected, actual)
}

func assertSliceEqual[T comparable](t *testing.T, expected []T, actual []T) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("expected (%+v) is not equal to actual (%+v): len(expected)=%d len(actual)=%d",
			expected, actual, len(expected), len(actual))
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("expected[%d] (%+v) is not equal to actual[%d] (%+v)",
				i, expected[i],
				i, actual[i])
		}
	}
}
