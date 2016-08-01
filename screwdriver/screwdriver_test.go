package screwdriver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func makeFakeHTTPClient(code int, body string, validator func(req *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validator(r)
		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func validateHeader(t *testing.T, key, value string) func(r *http.Request) {
	return func(r *http.Request) {
		headers, ok := r.Header[key]
		if !ok {
			t.Fatalf("No %s header sent in Screwdriver request", key)
		}
		header := headers[0]
		if header != value {
			t.Errorf("%s header = %q, want %q", key, header, value)
		}
	}
}

func TestFromBuildId(t *testing.T) {
	want := Build{
		ID:    "testId",
		JobID: "testJob",
	}
	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	wantToken := "faketoken"
	wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

	validatorFunc := validateHeader(t, "Authorization", wantTokenHeader)
	http := makeFakeHTTPClient(200, string(json), validatorFunc)

	testAPI := api{"http://fakeurl", wantToken, http}
	build, err := testAPI.BuildFromID(want.ID)

	if err != nil {
		t.Errorf("Unexpected error from BuildFromID: %v", err)
	}

	if build != want {
		t.Errorf("build == %#v, want %#v", build, want)
	}
}
